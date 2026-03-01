package cmd

import (
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// Curate runs the Curator pipeline on demand: checks single-target proposals
// for already-applied status, then clusters and synthesizes multi-target groups.
func Curate(args []string) error {
	fs := flag.NewFlagSet("curate", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would be curated without making changes")
	fs.Parse(args)

	proposals, err := pipeline.ListProposals()
	if err != nil {
		return fmt.Errorf("listing proposals: %w", err)
	}
	if len(proposals) == 0 {
		fmt.Println("No pending proposals to curate.")
		return nil
	}

	multi, single := pipeline.GroupProposalsByTarget(proposals)

	fmt.Println(cli.Bold("Curate Proposals"))
	fmt.Println(cli.Muted("════════════════════════════════════════"))
	fmt.Println()
	fmt.Printf("  %s  %d pending proposals\n", cli.Bold("Total:"), len(proposals))
	fmt.Printf("  %s  %d (Haiku batch check: already applied?)\n", cli.Bold("Single-target:"), len(single))

	multiCount := 0
	for _, g := range multi {
		multiCount += len(g)
	}
	fmt.Printf("  %s  %d across %d targets (Sonnet Curator: cluster + synthesize)\n",
		cli.Bold("Multi-target:"), multiCount, len(multi))
	fmt.Println()

	if *dryRun {
		if len(multi) > 0 {
			fmt.Println(cli.Bold("Multi-proposal targets:"))
			for target, group := range multi {
				fmt.Printf("  %s (%d proposals)\n", cli.Muted(target), len(group))
			}
			fmt.Println()
		}
		fmt.Println(cli.Muted("Dry run — no changes made."))
		return nil
	}

	// Initialize the invoke semaphore for CLI usage.
	cfg := pipeline.DefaultPipelineConfig()
	pipeline.InitInvokeSemaphore(cfg.MaxConcurrentInvocations)

	runStart := time.Now()
	var allDecisions []pipeline.CuratorDecision
	var curatorUsages []pipeline.InvocationUsage
	var checkUsage *pipeline.InvocationUsage

	// Stage 1: Haiku batch check for single-target proposals.
	if len(single) > 0 {
		fmt.Printf("Stage 1: checking %d single-target proposals...", len(single))
		checkDecisions, cr, err := pipeline.RunCuratorCheck(single, cfg)
		if err != nil {
			fmt.Printf(" %s\n", cli.Error(fmt.Sprintf("failed: %v", err)))
		} else {
			if cr != nil {
				u := pipeline.InvocationUsageFromResult(cr)
				checkUsage = &u
			}
			autoRejected := 0
			for _, cd := range checkDecisions {
				if cd.AlreadyApplied {
					reason := "auto-culled: already applied to target"
					if archErr := apply.Archive(cd.ProposalID, apply.OutcomeAutoRejected, reason); archErr != nil {
						fmt.Printf("\n  %s archiving %s: %v\n", cli.Error("error"), cd.ProposalID, archErr)
						continue
					}
					allDecisions = append(allDecisions, pipeline.CuratorDecision{
						ProposalID: cd.ProposalID,
						Action:     "auto-reject",
						Reason:     reason,
					})
					autoRejected++
				} else {
					allDecisions = append(allDecisions, pipeline.CuratorDecision{
						ProposalID: cd.ProposalID,
						Action:     "keep",
						Reason:     "single proposal, not already applied",
					})
				}
			}
			fmt.Printf(" done (%d already applied, %d kept)\n", autoRejected, len(checkDecisions)-autoRejected)
		}
	}

	// Stage 2: Sonnet Curator for multi-proposal targets (parallelized).
	if len(multi) > 0 {
		fmt.Printf("Stage 2: curating %d multi-proposal targets...\n", len(multi))

		type curatorResult struct {
			target   string
			manifest *pipeline.CuratorManifest
			cr       *pipeline.ClaudeResult
			err      error
		}

		resultsCh := make(chan curatorResult, len(multi))
		var wg sync.WaitGroup

		for target, group := range multi {
			wg.Add(1)
			go func(t string, g []pipeline.ProposalWithSession) {
				defer wg.Done()
				manifest, cr, err := pipeline.RunCuratorGroup(t, g, cfg)
				resultsCh <- curatorResult{target: t, manifest: manifest, cr: cr, err: err}
			}(target, group)
		}

		wg.Wait()
		close(resultsCh)

		for res := range resultsCh {
			if res.err != nil {
				fmt.Printf("  %s %s: %v\n", cli.Error("error"), res.target, res.err)
				continue
			}
			if res.cr != nil {
				curatorUsages = append(curatorUsages, pipeline.InvocationUsageFromResult(res.cr))
			}
			if res.manifest == nil {
				continue
			}

			if err := applyManifest(res.manifest); err != nil {
				fmt.Printf("  %s applying manifest for %s: %v\n", cli.Error("error"), res.target, err)
				continue
			}
			allDecisions = append(allDecisions, res.manifest.Decisions...)

			kept, culled, synthesized := 0, 0, 0
			for _, d := range res.manifest.Decisions {
				switch d.Action {
				case "keep":
					kept++
				case "cull", "auto-reject":
					culled++
				case "synthesize":
					synthesized++
				}
			}
			fmt.Printf("  %s: %d kept, %d culled, %d synthesized\n",
				cli.Muted(res.target), kept, culled, synthesized)
		}
	}

	// Summary.
	after, _ := pipeline.ListProposals()
	archived := 0
	synthesized := 0
	for _, dec := range allDecisions {
		switch dec.Action {
		case "cull", "auto-reject":
			archived++
		case "synthesize":
			synthesized++
		}
	}

	duration := time.Since(runStart)

	fmt.Println()
	fmt.Printf("%s  %d → %d proposals (%d archived, %d synthesized) in %s\n",
		cli.Bold("Done:"), len(proposals), len(after), archived, synthesized,
		duration.Round(time.Second))

	// Append cleanup record.
	rec := pipeline.CleanupRecord{
		Timestamp:       runStart,
		DurationNs:      int64(duration),
		ProposalsBefore: len(proposals),
		ProposalsAfter:  len(after),
		Decisions:       allDecisions,
		CuratorUsage:    curatorUsages,
		CheckUsage:      checkUsage,
	}
	if err := pipeline.AppendCleanupHistory(rec); err != nil {
		return fmt.Errorf("appending cleanup history: %w", err)
	}

	return nil
}

// applyManifest archives culled/rejected proposals and writes synthesized proposals.
func applyManifest(manifest *pipeline.CuratorManifest) error {
	for _, dec := range manifest.Decisions {
		switch dec.Action {
		case "cull":
			reason := "auto-culled: " + dec.Reason
			if err := apply.Archive(dec.ProposalID, apply.OutcomeCulled, reason); err != nil {
				return err
			}
		case "auto-reject":
			reason := "auto-culled: " + dec.Reason
			if err := apply.Archive(dec.ProposalID, apply.OutcomeAutoRejected, reason); err != nil {
				return err
			}
		case "synthesize":
			reason := "auto-culled: synthesized into " + dec.SupersededBy
			if err := apply.Archive(dec.ProposalID, apply.OutcomeCulled, reason); err != nil {
				return err
			}
		}
	}

	for _, cluster := range manifest.Clusters {
		if cluster.Synthesis == nil {
			continue
		}
		if err := pipeline.WriteProposal(cluster.Synthesis, "curator"); err != nil {
			return err
		}
	}

	return nil
}

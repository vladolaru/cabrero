package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// groupProposalsByTarget separates proposals into:
//   - multi: targets with 2+ proposals (map from target → proposals)
//   - single: file-target proposals that are the only proposal for their target
//
// Scaffolds in multi targets are moved to single (they are never curated).
// Non-file targets are excluded from single (no "already applied?" check possible).
func groupProposalsByTarget(proposals []pipeline.ProposalWithSession) (
	multi map[string][]pipeline.ProposalWithSession,
	single []pipeline.ProposalWithSession,
) {
	byTarget := make(map[string][]pipeline.ProposalWithSession)
	for _, pw := range proposals {
		byTarget[pw.Proposal.Target] = append(byTarget[pw.Proposal.Target], pw)
	}

	multi = make(map[string][]pipeline.ProposalWithSession)
	for target, group := range byTarget {
		// Split scaffolds out regardless of group size.
		var nonScaffold, scaffold []pipeline.ProposalWithSession
		for _, pw := range group {
			if pw.Proposal.Type == "skill_scaffold" {
				scaffold = append(scaffold, pw)
			} else {
				nonScaffold = append(nonScaffold, pw)
			}
		}
		// Scaffolds always go to single (kept as-is, no curation).
		for _, pw := range scaffold {
			if pipeline.IsFileTarget(pw.Proposal.Target) {
				single = append(single, pw)
			}
		}
		if len(nonScaffold) >= 2 {
			multi[target] = nonScaffold
		} else if len(nonScaffold) == 1 {
			if pipeline.IsFileTarget(nonScaffold[0].Proposal.Target) {
				single = append(single, nonScaffold[0])
			}
		}
	}
	return multi, single
}

// performCleanup runs the daily proposal cleanup:
//  1. Batched Haiku check for all single-proposal file-target proposals.
//  2. Parallelized Sonnet Curator for each multi-proposal target group.
//  3. Archives culled/rejected proposals and writes synthesized proposals.
//  4. Appends a CleanupRecord to cleanup_history.jsonl.
//  5. Sends a macOS notification with the summary.
func (d *Daemon) performCleanup(ctx context.Context) {
	runStart := time.Now()

	proposals, err := pipeline.ListProposals()
	if err != nil {
		d.log.Error("cleanup: listing proposals: %v", err)
		return
	}
	if len(proposals) == 0 {
		d.log.Info("cleanup: no proposals to process")
		return
	}

	d.log.Info("cleanup: starting (%d proposals)", len(proposals))

	multi, single := groupProposalsByTarget(proposals)

	var allDecisions []pipeline.CuratorDecision
	var curatorUsages []pipeline.InvocationUsage
	var checkUsage *pipeline.InvocationUsage

	// Stage 1: Haiku batch check for single-proposal targets.
	if len(single) > 0 {
		checkDecisions, cr, err := pipeline.RunCuratorCheck(single, d.config.Pipeline)
		if err != nil {
			d.log.Error("cleanup: curator check failed: %v", err)
			// Non-fatal: continue with multi-proposal curation.
		} else {
			if cr != nil {
				u := pipeline.InvocationUsageFromResult(cr)
				checkUsage = &u
			}
			// Apply check decisions.
			for _, cd := range checkDecisions {
				if cd.AlreadyApplied {
					reason := "auto-culled: already applied to target"
					if archErr := apply.Archive(cd.ProposalID, reason); archErr != nil {
						d.log.Error("cleanup: archiving %s: %v", cd.ProposalID, archErr)
						continue
					}
					allDecisions = append(allDecisions, pipeline.CuratorDecision{
						ProposalID: cd.ProposalID,
						Action:     "auto-reject",
						Reason:     reason,
					})
				} else {
					allDecisions = append(allDecisions, pipeline.CuratorDecision{
						ProposalID: cd.ProposalID,
						Action:     "keep",
						Reason:     "single proposal, not already applied",
					})
				}
			}
		}
	}

	// Stage 2: Sonnet Curator for multi-proposal targets (parallelized).
	if len(multi) > 0 {
		type curatorResult struct {
			target   string
			manifest *pipeline.CuratorManifest
			cr       *pipeline.ClaudeResult
			err      error
		}

		resultsCh := make(chan curatorResult, len(multi))
		var wg sync.WaitGroup

		for target, group := range multi {
			select {
			case <-ctx.Done():
				break
			default:
			}

			wg.Add(1)
			go func(t string, g []pipeline.ProposalWithSession) {
				defer wg.Done()
				manifest, cr, err := pipeline.RunCuratorGroup(t, g, d.config.Pipeline)
				resultsCh <- curatorResult{target: t, manifest: manifest, cr: cr, err: err}
			}(target, group)
		}

		wg.Wait()
		close(resultsCh)

		for res := range resultsCh {
			if res.err != nil {
				d.log.Error("cleanup: curator for %s: %v", res.target, res.err)
				continue
			}
			if res.cr != nil {
				curatorUsages = append(curatorUsages, pipeline.InvocationUsageFromResult(res.cr))
			}
			if res.manifest == nil {
				continue
			}

			// Apply manifest decisions.
			if err := d.applyManifest(res.manifest); err != nil {
				d.log.Error("cleanup: applying manifest for %s: %v", res.target, err)
				continue
			}
			allDecisions = append(allDecisions, res.manifest.Decisions...)
			d.log.Info("cleanup: target %s — %d decisions", res.target, len(res.manifest.Decisions))
		}
	}

	// Count outcome.
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
	d.log.Info("cleanup: complete in %s — %d→%d proposals (%d archived, %d synthesized new)",
		duration.Round(time.Second), len(proposals), len(after), archived, synthesized)

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
		d.log.Error("cleanup: appending history: %v", err)
	}

	// Notify.
	msg := fmt.Sprintf("Cleanup: %d→%d proposals (%d archived)", len(proposals), len(after), archived)
	if err := d.notify("Cabrero", msg); err != nil {
		d.log.Error("cleanup notification failed: %v", err)
	}
}

// applyManifest archives culled/rejected proposals and writes synthesized proposals.
func (d *Daemon) applyManifest(manifest *pipeline.CuratorManifest) error {
	// Process decisions.
	for _, dec := range manifest.Decisions {
		switch dec.Action {
		case "cull":
			reason := "auto-culled: " + dec.Reason
			if err := apply.Archive(dec.ProposalID, reason); err != nil {
				d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
			}
		case "auto-reject":
			reason := "auto-culled: " + dec.Reason
			if err := apply.Archive(dec.ProposalID, reason); err != nil {
				d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
			}
		case "synthesize":
			reason := "auto-culled: synthesized into " + dec.SupersededBy
			if err := apply.Archive(dec.ProposalID, reason); err != nil {
				d.log.Error("cleanup: archiving %s: %v", dec.ProposalID, err)
			}
		case "keep":
			// No action needed.
		}
	}

	// Write synthesized proposals from clusters.
	for _, cluster := range manifest.Clusters {
		if cluster.Synthesis == nil {
			continue
		}
		if err := pipeline.WriteProposal(cluster.Synthesis, "curator"); err != nil {
			d.log.Error("cleanup: writing synthesized proposal %s: %v", cluster.Synthesis.ID, err)
		}
	}

	return nil
}

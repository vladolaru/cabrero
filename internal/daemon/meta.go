package daemon

import (
	"context"
	"fmt"
	"math"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

// performMetaRun runs Stage 1 (metric computation) unconditionally.
// Stage 2 (Opus meta-analysis) fires only when a threshold is crossed with
// sufficient samples and no recent meta-proposal exists.
func (d *Daemon) performMetaRun(ctx context.Context) {
	d.log.Info("meta: computing pipeline metrics")

	metrics, err := pipeline.ComputePipelineMetrics(d.config.Pipeline)
	if err != nil {
		d.log.Error("meta: computing metrics: %v", err)
		return
	}

	cfg := d.config.Pipeline
	triggered := false

	// Check classifier FPR threshold.
	if !math.IsNaN(metrics.ClassifierFPR) &&
		metrics.ClassifierFPR >= cfg.MetaClassifierFPRThreshold {
		d.log.Info("meta: classifier FPR %.0f%% exceeds threshold %.0f%%",
			metrics.ClassifierFPR*100, cfg.MetaClassifierFPRThreshold*100)
		triggered = true
	}

	// Check per-version rejection rate.
	for _, stats := range metrics.AcceptanceByVersion {
		if stats.SampleSize < cfg.MetaMinSamples {
			continue
		}
		rejectionRate := 1.0 - stats.AcceptanceRate
		if math.IsNaN(rejectionRate) || rejectionRate < cfg.MetaRejectionRateThreshold {
			continue
		}
		if metaCooldownActive(stats.PromptVersion, cfg.MetaCooldownDays) {
			d.log.Info("meta: version %s above threshold but in cooldown period, skipping",
				stats.PromptVersion)
			_ = pipeline.AppendMetaRecord(pipeline.HistoryStatusMetaCooldown,
				fmt.Sprintf("version %s in cooldown", stats.PromptVersion))
			continue
		}
		d.log.Info("meta: version %s rejection rate %.0f%% exceeds threshold %.0f%%",
			stats.PromptVersion, rejectionRate*100, cfg.MetaRejectionRateThreshold*100)
		triggered = true

		propID, err := pipeline.RunMetaAnalysis(stats, cfg)
		if err != nil {
			d.log.Error("meta: RunMetaAnalysis for %s: %v", stats.PromptVersion, err)
			continue
		}
		if propID != "" {
			d.log.Info("meta: proposal %s written for version %s", propID, stats.PromptVersion)
			_ = pipeline.AppendMetaRecord(pipeline.HistoryStatusMetaTriggered,
				fmt.Sprintf("version %s, proposal %s", stats.PromptVersion, propID))
		}
	}

	if !triggered {
		fprStr := "n/a"
		if !math.IsNaN(metrics.ClassifierFPR) {
			fprStr = fmt.Sprintf("%.0f%%", metrics.ClassifierFPR*100)
		}
		d.log.Info("meta: no thresholds exceeded (classifier FPR: %s, %d versions checked)",
			fprStr, len(metrics.AcceptanceByVersion))
		_ = pipeline.AppendMetaRecord(pipeline.HistoryStatusMetaNoThreshold,
			fmt.Sprintf("classifier FPR: %s, versions: %d", fprStr, len(metrics.AcceptanceByVersion)))
	}
}

// metaCooldownActive returns true if a prompt_improvement proposal for the
// given version was created within the last cooldownDays. Checks both pending
// proposals and archived proposals (approved/rejected) to prevent re-triggering
// shortly after a recent meta decision.
func metaCooldownActive(promptVersion string, cooldownDays int) bool {
	cutoff := pipeline.MetaCooldownCutoff(cooldownDays)

	// Check pending proposals.
	proposals, err := pipeline.ListProposals()
	if err == nil {
		for _, pw := range proposals {
			if pw.Proposal.Type != pipeline.TypePromptImprovement {
				continue
			}
			if pw.Proposal.Target == promptVersion || pw.Proposal.Rationale == promptVersion {
				if pipeline.ProposalCreatedAfter(pw, cutoff) {
					return true
				}
			}
		}
	}

	// Check archived proposals (recently approved/rejected).
	if pipeline.ArchivedMetaProposalAfter(promptVersion, cutoff) {
		return true
	}

	return false
}

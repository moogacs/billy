package analyze

import (
	"github.com/geekmonkey/billy/internal/model"
	"github.com/geekmonkey/billy/internal/pricing"
	"github.com/geekmonkey/billy/internal/turnbuilder"
)

// BuildReport converts normalized events into priced turns and a flat answer list.
func BuildReport(sessionPath string, vendor model.Vendor, events []model.NormalizedEvent, pt *pricing.Table) model.AnalyzeReport {
	turns := turnbuilder.BuildTurns(events)
	disclaimer := pt.DisclaimerLine()

	var turnReports []model.TurnReport
	var answers []model.AnswerReport

	for _, t := range turns {
		var turnUsage model.UsageBreakdown
		var turnCost float64
		var turnAnswers []model.AnswerReport

		for _, c := range t.Completions {
			cost, ok := pt.CostUSD(c.Vendor, c.Model, c.Usage)
			ar := model.AnswerReport{
				TurnIndex:    t.Index,
				Model:        c.Model,
				Timestamp:    c.Timestamp,
				DedupKey:     c.DedupKey,
				Usage:        c.Usage,
				CostUSD:      cost,
				UnknownModel: !ok,
				SourceRef:    c.SourceRef,
			}
			answers = append(answers, ar)
			turnAnswers = append(turnAnswers, ar)
			turnUsage = turnUsage.Add(c.Usage)
			turnCost += cost
		}

		turnReports = append(turnReports, model.TurnReport{
			Index:           t.Index,
			Prompt:          t.Prompt.Text,
			PromptTimestamp: t.Prompt.Timestamp,
			Usage:           turnUsage,
			CostUSD:         turnCost,
			Answers:         turnAnswers,
		})
	}

	return model.AnalyzeReport{
		Meta: model.ReportMeta{
			Vendor:      vendor,
			SessionPath: sessionPath,
			Disclaimer:  disclaimer,
		},
		Turns:   turnReports,
		Answers: answers,
	}
}

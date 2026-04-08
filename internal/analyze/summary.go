package analyze

import "github.com/geekmonkey/billy/internal/model"

// SumAnswers aggregates cost, token usage, and counts from priced answers.
func SumAnswers(rep model.AnalyzeReport) (cost float64, usage model.UsageBreakdown, answers int, unknownModels int) {
	for _, a := range rep.Answers {
		cost += a.CostUSD
		usage = usage.Add(a.Usage)
		answers++
		if a.UnknownModel {
			unknownModels++
		}
	}
	return cost, usage, answers, unknownModels
}

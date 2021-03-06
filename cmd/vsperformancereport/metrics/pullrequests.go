package metrics

import (
	"fmt"
	"github.com/gocarina/gocsv"
	"github.com/montanaflynn/stats"
	"github.com/urfave/cli/v2"
	"math"
	"os"
	"strings"
	"time"
)

type PullRequestPerformanceMetric struct {
	Owner     string    `csv:"owner"`
	Repo      string    `csv:"repo"`
	CreatedAt time.Time `csv:"created_at"`
	Merged    bool      `csv:"merged"`
	// Duration will use time to merged, if not will use
	// time to cosed
	DurationSeconds    float64 `csv:"duration"`
	Comments           int     `csv:"comments"`
	Additions          int     `csv:"additions"`
	Deletions          int     `csv:"deletions"`
	TotalChanges       int     `csv:"total_changes"`
	DurationPerComment float64 `csv:"duration_per_comment"`
	DurationPerLine    float64 `csv:"duration_per_line"`
}

type PullRequestPerformanceAggregate struct {
	Key                          string
	Interval                     string
	Owner                        string
	Repo                         string
	TotalPullRequests            int
	NumMerged                    int
	MergeRatio                   float64
	AvgTotalLinesChanged         float64
	AvgDurationHours             float64
	AvgDurationSecondsPerLine    float64
	AvgDurationSecondsPerComment float64
}

func intervalToKey(i string, createdAt time.Time) (string, error) {
	switch i {
	case "day":
		year, month, day := createdAt.Date()
		return fmt.Sprintf("%d|%d|%d", year, month, day), nil
	case "week":
		year, week := createdAt.ISOWeek()
		return fmt.Sprintf("%d|%d", year, week), nil
	case "month":
		year, month, _ := createdAt.Date()
		return fmt.Sprintf("%d|%d", year, month), nil
	}
	return "", fmt.Errorf("interval: %s not supported", i)
}

func NewPullRequestPerformanceAggregation(aggInterval string, ms []PullRequestPerformanceMetric) ([]PullRequestPerformanceAggregate, error) {
	// by default aggregate by week
	bucketed := make(map[string][]PullRequestPerformanceMetric)

	for _, pr := range ms {
		intervalKey, err := intervalToKey(aggInterval, pr.CreatedAt)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf(
			"%s_%s|%s",
			intervalKey,
			pr.Owner,
			pr.Repo,
		)
		bucketed[key] = append(bucketed[key], pr)
	}

	var aggs []PullRequestPerformanceAggregate

	for key, metrics := range bucketed {
		var numMerged int

		agg := PullRequestPerformanceAggregate{
			Interval:          strings.Split(key, "_")[0],
			Key:               key,
			Owner:             metrics[0].Owner,
			Repo:              metrics[0].Repo,
			TotalPullRequests: len(metrics),
		}

		var durations []float64
		var durationsPerLine []float64
		var durationsPerComment []float64
		var totalLinesChange []float64
		for _, m := range metrics {
			durations = append(durations, m.DurationSeconds)
			durationsPerLine = append(durationsPerLine, m.DurationPerLine)
			durationsPerComment = append(durationsPerComment, m.DurationPerComment)
			totalLinesChange = append(totalLinesChange, float64(m.TotalChanges))

			if m.Merged {
				numMerged++
			}
		}

		// calc the % Merged
		agg.NumMerged = numMerged
		agg.MergeRatio = math.Round(
			(float64(agg.NumMerged)/float64(agg.TotalPullRequests))*100,
		) / 100

		// calc average duration
		avgDuration, err := stats.Mean(durations)

		if err != nil {
			return nil, err
		}
		agg.AvgDurationHours = avgDuration / (60 * 60) // 60 seconds / 1 minute * 60 minutes / 1 hour

		/*
			// calc p95 duration
			p95Duration, err := stats.Percentile(durations, 0.95)
			if err != nil {
				return nil, err
			}

			agg.P95Duration = p95Duration
		*/

		// calc avg per line
		avgDurationPerLine, err := stats.Mean(durationsPerLine)
		if err != nil {
			return nil, err
		}
		agg.AvgDurationSecondsPerLine = avgDurationPerLine

		// calc avg per comment
		avgDurationPerComment, err := stats.Mean(durationsPerComment)
		if err != nil {
			return nil, err
		}
		agg.AvgDurationSecondsPerComment = avgDurationPerComment

		// calc avg total lines changed per pull request
		avgTotalLinesChanged, err := stats.Mean(totalLinesChange)
		if err != nil {
			return nil, err
		}
		agg.AvgTotalLinesChanged = avgTotalLinesChanged

		aggs = append(aggs, agg)
	}

	return aggs, nil
}

func NewPullRequestAggregation() *cli.Command {
	return &cli.Command{
		Name:  "agg",
		Usage: "generate aggregates from raw data",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "in",
				Value: "",
				Usage: "the raw pull request information file as CSV",
			},
			&cli.StringFlag{
				Name:  "agg-window",
				Value: "week",
				Usage: "the raw pull request information file as CSV, supports (day|week|month)",
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "pull-request",
				Usage: "generate aggregates from raw pull_request data",
				Action: func(c *cli.Context) error {
					f, err := os.Open(c.String("in"))
					if err != nil {
						return err
					}
					defer f.Close()
					var ms []PullRequestPerformanceMetric
					if err := gocsv.UnmarshalFile(f, &ms); err != nil {
						return err
					}

					aggs, err := NewPullRequestPerformanceAggregation(c.String("agg-window"), ms)
					if err != nil {
						return err
					}

					csvString, err := gocsv.MarshalString(aggs)
					if err != nil {
						return err
					}

					fmt.Println(csvString)

					return nil
				},
			},
		},
	}
}

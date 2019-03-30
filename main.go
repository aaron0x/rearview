package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"
)

type datePrice struct {
	Date      time.Time
	HighPrice float64
}

type logger struct {
	verbose bool
}

func (l *logger) Tracef(format string, v ...interface{}) {
	if l.verbose {
		fmt.Printf(format, v...)
	}
}

func (l *logger) Printf(format string, v ...interface{}) {
	fmt.Printf(format, v...)
}

func newLogger(verbose bool) *logger {
	return &logger{
		verbose,
	}
}

type config struct {
	capital       int64
	run           int
	yearPerRun    int
	inflationRate float64
	costPerYear   int
}

func main() {
	verbose := flag.Bool("v", false, "show verbose progress")
	capital := flag.Int64("c", 333333, "initial capital")
	filePath := flag.String("f", "./GSPC.csv", "input csv path")
	run := flag.Int("r", 5, "how many runs to test")
	yearPerRun := flag.Int("y", 10, "how many years in one run")
	inflationRate := flag.Float64("i", 1.016, "inflation rate")
	costPerYear := flag.Int("l", 16666, "cost per year")
	flag.Parse()

	logger := newLogger(*verbose)
	config := config{
		capital:       *capital,
		run:           *run,
		yearPerRun:    *yearPerRun,
		inflationRate: *inflationRate,
		costPerYear:   *costPerYear,
	}

	file, err := os.Open(*filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	datePrices, err := parseCSVFile(file)
	if err != nil {
		panic(err)
	}
	if len(datePrices) == 0 {
		panic("no input data")
	}

	checkStrategy(&config, datePrices, logger)
}

func findClosestDay(day time.Time, inDatePrices []*datePrice) (int, bool) {
	index := sort.Search(len(inDatePrices), func(i int) bool {
		datePriceDate := inDatePrices[i].Date
		return datePriceDate.After(day) || datePriceDate.Equal(day)
	})
	if index == len(inDatePrices) {
		return -1, false
	}
	return index, true
}

const (
	success = iota
	failed
	na
)

func checkStrategy(config *config, datePrices []*datePrice, logger *logger) {
	successCount, failedCount, naCount := 0, 0, 0
	for i := range datePrices {
		r := checkInPeriod(config, datePrices[i:], logger)

		switch r {
		case success:
			successCount++
		case failed:
			failedCount++
		case na:
			naCount++
		default:
			panic(fmt.Sprintf("unknow check result %d", r))
		}
	}
	logger.Printf("success %d, failed: %d, N/A: %d, successful rate %f\n", successCount, failedCount, naCount, float64(successCount)/float64(successCount+failedCount))
}

func toyyyymmdd(date time.Time) string {
	return date.Format("2006-01-02")
}

func checkInPeriod(config *config, datePrices []*datePrice, logger *logger) int {
	// initial shares
	heldShares := int64(float64(config.capital) / datePrices[0].HighPrice)
	logger.Tracef("initial: capital %d, it can buy %d shares\n\n", config.capital, heldShares)

	startDay, endDay := datePrices[0].Date, datePrices[0].Date
	for run := 0; run < config.run; run++ {
		// find index of start day and end day in datePrices for this run
		startDay, endDay = endDay, endDay.AddDate(config.yearPerRun, 0, 0)
		startIndex, sFound := findClosestDay(startDay, datePrices)
		endIndex, eFound := findClosestDay(endDay, datePrices)
		if !sFound || !eFound {
			logger.Tracef("no more available date to test\n")
			return na
		}

		// compute captial after this run and cost of live with inflation considered
		// add these two then we have target capital in this run
		inflationRate := math.Pow(config.inflationRate, float64((run+1)*config.yearPerRun))
		inflationCapital := float64(config.capital) * inflationRate
		costOfLiving := float64(config.costPerYear) * float64(config.yearPerRun) * inflationRate
		targetCapital := inflationCapital + costOfLiving
		logger.Tracef("%s to %s, target capital %d, prepared cost of living %d\n",
			toyyyymmdd(datePrices[startIndex].Date),
			toyyyymmdd(datePrices[endIndex].Date),
			int(targetCapital),
			int(costOfLiving),
		)

		satisfied := false
		for currIndex := startIndex; currIndex < endIndex; currIndex++ {
			// find one day in this run satisfy our target captial
			datePrice := datePrices[currIndex]
			if float64(heldShares)*datePrice.HighPrice >= targetCapital {
				satisfied = true

				// sold shares to get money ^^
				soldShares := int64(costOfLiving / datePrice.HighPrice)
				heldShares -= soldShares

				logger.Tracef("%s sell %d shares in %f, earn %d, remained shares %d\n",
					toyyyymmdd(datePrice.Date),
					soldShares,
					datePrice.HighPrice,
					int64(float64(soldShares)*datePrice.HighPrice),
					heldShares,
				)
				logger.Tracef("new capital %d\n\n", int(float64(heldShares)*datePrice.HighPrice))
				break
			}
		}

		if !satisfied {
			logger.Tracef("not satisfied\n")
			return failed
		}
	}

	return success
}

// expected column order:
// Date Open High
// It's the format yahoo finace provided
func parseCSVFile(file *os.File) ([]*datePrice, error) {
	reader := csv.NewReader(file)

	// skip column name
	_, err := reader.Read()
	if err != nil {
		return nil, err
	}

	datePrices := []*datePrice{}
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		year, month, day := 0, 0, 0
		fmt.Sscanf(line[0], "%d-%d-%d", &year, &month, &day)

		highPrice := float64(0)
		fmt.Sscanf(line[2], "%f", &highPrice)

		datePrice := datePrice{
			Date:      time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
			HighPrice: highPrice,
		}
		datePrices = append(datePrices, &datePrice)
	}

	return datePrices, nil
}

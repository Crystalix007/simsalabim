package main

import (
	"bufio"
	"container/heap"
	"flag"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	FFTChunkSeconds float64 = 0.3
)

func main() {
	var filenames []string
	flag.Parse()

	for _, filename := range flag.Args() {
		filenames = append(filenames, filename)
	}

	if len(filenames) != 2 {
		log.Fatalf("Expected the file names of two fingerprints to compare")
	}

	var files []*bufio.Scanner

	for _, filename := range filenames {
		f, err := os.Open(filename)
		defer f.Close()

		if err != nil {
			log.Fatalf("Could not open file '%s'", filename)
		}

		files = append(files, bufio.NewScanner(f))
	}

	var fileTimestamps [][]TimestampFingerprint = [][]TimestampFingerprint{}

	for i, file := range files {
		fileTimestamps = append(fileTimestamps, []TimestampFingerprint{})

		for file.Scan() {
			var timestamp TimestampFingerprint

			lineScanner := bufio.NewScanner(strings.NewReader(file.Text()))
			lineScanner.Split(bufio.ScanWords)

			if !lineScanner.Scan() {
				log.Fatalf("Incorrectly formatted fingerprint line: '%s'", file.Text())
			}

			lineTimestamp, err := strconv.ParseFloat(lineScanner.Text(), 64)
			timestamp.timestamp = lineTimestamp

			if err != nil {
				log.Fatalf("Incorrectly formatted fingerprint timestamp: '%s'", lineScanner.Text())
			}

			for lineScanner.Scan() {
				frequency, err := strconv.ParseFloat(lineScanner.Text(), 64)

				if err != nil {
					log.Fatalf("Incorrectly formatted fingerprint frequency: '%s'", lineScanner.Text())
				}

				timestamp.frequency = append(timestamp.frequency, frequency)
			}

			fileTimestamps[i] = append(fileTimestamps[i], timestamp)
		}
	}

	res := compareTimestampFingerprints(fileTimestamps)

	log.Printf("Loss: %f\n", res.globalLoss)
}

type TimestampFingerprint struct {
	frequency []float64
	timestamp float64
}

func (tf1 TimestampFingerprint) Loss(tf2 TimestampFingerprint) float64 {
	if len(tf1.frequency) != len(tf2.frequency) {
		panic("Fingerprints do not have matching frequency sample counts")
	}

	var loss float64 = math.MaxFloat64

	sortedFs1 := tf1.frequency
	sort.Float64s(sortedFs1)
	sortedFs2 := tf2.frequency
	sort.Float64s(sortedFs2)

	for i := range sortedFs1 {
		loss = math.Min(loss, math.Abs(sortedFs1[i] - sortedFs2[i]))
	}

	return loss
}

type CompareResult struct {
	globalLoss float64
}

type FingerprintLoss struct {
	offset float64
	loss float64
}

func (fl FingerprintLoss) Less(fl2 FingerprintLoss) bool {
	return fl.loss < fl2.loss
}

type FingerprintsLoss []*FingerprintLoss

func (fl FingerprintsLoss) Len() int {
	return len(fl)
}

func (fl FingerprintsLoss) Swap(i, j int) {
	fl[i], fl[j] = fl[j], fl[i]
}

func (fl FingerprintsLoss) Less(i, j int) bool {
	return fl[i].Less(*fl[j])
}

func (fl *FingerprintsLoss) Push(x interface{}) {
	// a failed type assertion here will panic
	*fl = append(*fl, x.(*FingerprintLoss))
}

func (fl *FingerprintsLoss) Pop() interface{} {
	old := *fl
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*fl = old[0 : n-1]
	return item
}

func compareTimestampFingerprints(fingerprints [][]TimestampFingerprint) CompareResult {
	var res CompareResult = CompareResult{}

	if len(fingerprints) != 2 {
		panic("Cannot compare more than two sets of fingerprints")
	}

	window, reference := getFingerprintComparisonOrder(&fingerprints[0], &fingerprints[1])
	winLength := len(*window)
	refLength := len(*reference)

	fingerprintLosses := &FingerprintsLoss{}

	for i := range *reference {
		// If we don't have enough reference samples to compare the window against
		if refLength < winLength + i {
			break
		}

		var loss float64 = 0.0

		for j := range *window {
			loss += (*window)[j].Loss((*reference)[i + j])
		}

		heap.Push(fingerprintLosses, &FingerprintLoss{
			offset: (*reference)[i].timestamp,
			loss: loss,
		})
	}

	res.globalLoss = heap.Pop(fingerprintLosses).(*FingerprintLoss).loss / float64(winLength)

	return res
}

func getFingerprintComparisonOrder(fingerprint1, fingerprint2 *[]TimestampFingerprint) (*[]TimestampFingerprint, *[]TimestampFingerprint) {
	if len(*fingerprint1) < len(*fingerprint2) {
		return fingerprint1, fingerprint2
	}

	return fingerprint2, fingerprint1
}

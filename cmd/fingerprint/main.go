package main

import (
	"bufio"
	"container/heap"
	"flag"
	"fmt"
	"io"
	"log"
	"math/cmplx"
	"os"
	"runtime/pprof"

	"github.com/mewkiz/flac"
	"github.com/mjibson/go-dsp/fft"
)

const (
	FFTChunkSeconds float64 = 0.3
	FrequencyCount          = 3
)

func main() {
	var (
		filename string
		output   string
		profile  string
	)

	flag.StringVar(&filename, "filename", "", "audio file to fingerprint")
	flag.StringVar(&output, "output", "", "fingerprint output file")
	flag.StringVar(&profile, "profile", "", "profile performance and write to file")
	flag.Parse()

	if filename == "" {
		log.Fatal("Must specify filename of audio file to fingerprint")
	}

	audioStream, err := flac.Open(filename)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	defer audioStream.Close()

	var outputFile *bufio.Writer

	if output != "" {
		f, err := os.Create(output)

		if err != nil {
			log.Fatalf("%+v", err)
		}

		defer f.Close()

		outputFile = bufio.NewWriter(f)
	}

	if profile != "" {
		f, err := os.Create(profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	fingerprints, err := fingerprint(audioStream)

	if err != nil {
		log.Fatalf("%+v", err)
	}

	for _, fingerprint := range fingerprints {
		if outputFile != nil {
			fmt.Fprintf(outputFile, "%.2f ", fingerprint.timestamp)

			for _, freq := range fingerprint.frequencies {
				fmt.Fprintf(outputFile, "%f ", freq.frequency)
			}

			fmt.Fprintf(outputFile, "\n")
		} else {
			fmt.Printf("Fingerprint@%.2f:\n", fingerprint.timestamp)
			for _, freq := range fingerprint.frequencies {
				fmt.Printf("\t%.1fHz\t (%.3f)\n", freq.frequency, freq.magnitude)
			}
		}
	}

	if outputFile != nil {
		outputFile.Flush()
	}
}

func fingerprint(stream *flac.Stream) ([]*TimestampFingerprint, error) {
	sampleStream := make(chan float64)
	sampleRate := stream.Info.SampleRate
	fingerprintStream := fingerprintChan(sampleStream, sampleRate)
	fingerprints := []*TimestampFingerprint{}
	var err error = nil

	go func() {
		defer close(sampleStream)

		for {
			frame, err := stream.ParseNext()
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				break
			}

			var denominator int = 1 << frame.BitsPerSample

			for i := 0; i < frame.Subframes[0].NSamples; i++ {
				// TODO: Handle stereo

				sampleStream <- float64(frame.Subframes[0].Samples[i]) / float64(denominator)
				//for _, subframe := range frame.Subframes {
				//	sampleStream <- float64(subframe.Samples[i]) / float64(denominator)

				//	break
				//}
			}
		}
	}()

	for fingerprint := range fingerprintStream {
		fingerprints = append(fingerprints, fingerprint)
	}

	return fingerprints, err
}

type TimestampFingerprint struct {
	*Fingerprint
	timestamp float64
}

type Fingerprint struct {
	frequencies []FrequencyMagnitude
}

func (f Fingerprint) Timestamp(timestamp float64) *TimestampFingerprint {
	return &TimestampFingerprint{
		&f,
		timestamp,
	}
}

func fingerprintChan(stream <-chan float64, sampleRate uint32) <-chan *TimestampFingerprint {
	fingerprints := make(chan *TimestampFingerprint)
	var chunkSize = int(float64(sampleRate) * FFTChunkSeconds)

	go func() {
		defer close(fingerprints)
		var chunk = make([]float64, chunkSize)
		var timestamp float64 = 0
		var index = 0

		for sample := range stream {
			chunk[index] = sample
			index++

			if index == chunkSize {
				fingerprints <- fingerprintChunk(chunk, sampleRate).Timestamp(timestamp)

				timestamp += FFTChunkSeconds
				index = 0
			}
		}
	}()

	return fingerprints
}

type FrequencyMagnitude struct {
	frequency float64
	magnitude float64
}

func (f FrequencyMagnitude) Less(f2 FrequencyMagnitude) bool {
	return f.magnitude < f2.magnitude
}

type FrequencyQueue []*FrequencyMagnitude

func (fq FrequencyQueue) Len() int {
	return len(fq)
}

func (fq FrequencyQueue) Swap(i, j int) {
	fq[i], fq[j] = fq[j], fq[i]
}

func (fq FrequencyQueue) Less(i, j int) bool {
	return fq[j].Less(*fq[i])
}

func (fq *FrequencyQueue) Push(x interface{}) {
	// a failed type assertion here will panic
	*fq = append(*fq, x.(*FrequencyMagnitude))
}

func (fq *FrequencyQueue) Pop() interface{} {
	old := *fq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*fq = old[0 : n-1]
	return item
}

func fingerprintChunk(chunk []float64, sampleRate uint32) Fingerprint {
	spectrogram := fft.FFTReal(chunk)
	magnitudes := &FrequencyQueue{}

	for i := 0; i < len(spectrogram)/2-1; i++ {
		r := cmplx.Abs(spectrogram[i])

		heap.Push(magnitudes, &FrequencyMagnitude{
			frequency: fftIndexToFreq(i, sampleRate, len(spectrogram)),
			magnitude: r,
		})
	}

	return fingerprintFrequencies(magnitudes)
}

func fingerprintFrequencies(freqs *FrequencyQueue) Fingerprint {
	frequencies := make([]FrequencyMagnitude, FrequencyCount)

	for i := 0; i < FrequencyCount; i++ {
		frequencies[i] = *heap.Pop(freqs).(*FrequencyMagnitude)
	}

	return Fingerprint{
		frequencies: frequencies,
	}
}

func fftIndexToFreq(index int, sampleRate uint32, fftSize int) float64 {
	return float64(index) * float64(sampleRate) / float64(fftSize)
}

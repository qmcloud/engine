// Copyright 2014 The Azul3D Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flac

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/qmcloud/engine/audio"
)

type decodeTest struct {
	file         string
	samplesTotal int
	audio.Config
	start audio.Slice
}

func testDecode(t *testing.T, tst decodeTest) {
	// Open the file.
	file, err := os.Open(tst.file)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	// Create a decoder for the audio source
	decoder, format, err := audio.NewDecoder(file)
	if err != nil {
		t.Fatal(err)
	}

	// Check for a valid format identifier.
	if format != "flac" {
		t.Fatalf(`Incorrect format, want "flac" got %q\n`, format)
	}

	// Ensure the decoder's configuration is correct.
	config := decoder.Config()
	if config != tst.Config {
		t.Fatalf("Incorrect configuration, expected %+v, got %+v\n", tst.Config, config)
	}

	// Create a slice large enough to hold 1 second of audio samples.
	bufSize := 1 * config.SampleRate * config.Channels
	buf := tst.start.Make(bufSize, bufSize)

	// Read audio samples until there are no more.
	first := true
	var samplesTotal int
	for {
		read, err := decoder.Read(buf)
		samplesTotal += read
		if first {
			// Validate the audio samples.
			first = false
			for i := 0; i < tst.start.Len(); i++ {
				if buf.At(i) != tst.start.At(i) {
					t.Log("got", buf.Slice(0, tst.start.Len()))
					t.Log("want", tst.start)
					t.Fatal("Bad sample data.")
				}
			}
		}
		if err == audio.EOS {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Ensure that we read the correct number of samples.
	if samplesTotal != tst.samplesTotal {
		t.Fatalf("Read %d audio samples, expected %d.\n", samplesTotal, tst.samplesTotal)
	}
}

func TestDecodeUInt8(t *testing.T) {
	testDecode(t, decodeTest{
		file:         "testdata/tune_stereo_44100hz_uint8.flac",
		samplesTotal: 90524,
		Config: audio.Config{
			SampleRate: 44100,
			Channels:   2,
		},
		start: audio.Uint8{128, 128, 128, 128, 128, 128, 127, 127, 128, 128, 128, 128, 128, 127, 128},
	})
}

func TestDecodeInt16(t *testing.T) {
	testDecode(t, decodeTest{
		file:         "testdata/tune_stereo_44100hz_int16.flac",
		samplesTotal: 90524,
		Config: audio.Config{
			SampleRate: 44100,
			Channels:   2,
		},
		start: audio.Int16{0, 0, 0, 0, 1, 0, -1, 0, 1, -1, -1, 2, 3, -2, 0},
	})
}

func TestDecodeInt24(t *testing.T) {
	testDecode(t, decodeTest{
		file:         "testdata/tune_stereo_44100hz_int24.flac",
		samplesTotal: 90524,
		Config: audio.Config{
			SampleRate: 44100,
			Channels:   2,
		},
		start: audio.Int32{0, 0, 0, 0, 8, 0, 31, 0, 71, 0, 124, 1, 179, 2, 233},
	})
}

func benchDecode(b *testing.B, fmt audio.Slice, path string) {
	// Read the file into memory so we are strictly benchmarking the decoder,
	// avoiding disk read performance.
	data, err := ioutil.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	// Create a new decoder for the audio source to retrieve the configuration.
	decoder, _, err := audio.NewDecoder(bytes.NewReader(data))
	if err != nil {
		b.Fatal(err)
	}
	config := decoder.Config()

	// Create a slice of type fmt large enough to hold 1 second of audio
	// samples.
	bufSize := 1 * config.SampleRate * config.Channels
	buf := fmt.Make(bufSize, bufSize)

	// Decode the entire file b.N times.
	for i := 0; i < b.N; i++ {
		// Create a new decoder for the audio source
		decoder, _, err := audio.NewDecoder(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}

		// Read audio samples until there are no more.
		for {
			_, err := decoder.Read(buf)
			if err == audio.EOS {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkDecodeUint8(b *testing.B) {
	benchDecode(b, audio.Uint8{}, "testdata/tune_stereo_44100hz_uint8.flac")
}

func BenchmarkDecodeInt16(b *testing.B) {
	benchDecode(b, audio.Int16{}, "testdata/tune_stereo_44100hz_int16.flac")
}

func BenchmarkDecodeInt24(b *testing.B) {
	benchDecode(b, audio.Int32{}, "testdata/tune_stereo_44100hz_int24.flac")
}

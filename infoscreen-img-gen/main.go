package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

const title = "Lämpötilat"

var (
	envFile = map[string]string{}
)

type RawMeasurement struct {
	Label string
	Mac   string
	Value float32
	At    time.Time
}

type Measurement struct {
	Label     string
	Value     float32
	Slope     float32
	AgeInMins int
	Empty     bool
}

func (m *Measurement) FormatLabel() string {
	if m.AgeInMins > 0 {
		return fmt.Sprintf("%s", m.Label)
	}
	return m.Label
}

func (m *Measurement) FormatValue() string {
	if m.Empty {
		return "--"
	}
	return fmt.Sprintf("%.1f°", m.Value)
}

func loadConfiguration() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	envFile, err = godotenv.Read(path.Join(exPath, ".env"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}

	log.Debug().Msgf("Read environment %v", envFile)
}

func main() {

	output := os.Args[1]

	loadConfiguration()
	measurements := readData()
	drawResult(measurements, output)
}

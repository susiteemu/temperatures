package main

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

//go:embed configuration/*.yaml
var configuration embed.FS

const title = "Lämpötilat"

var (
	envFile = map[string]string{}
)

type GenerateImageConfiguration struct {
	Output string  `yaml:"output"`
	ImgW   int     `yaml:"image_width"`
	ImgH   int     `yaml:"image_height"`
	FontL  float64 `yaml:"font_large"`
	FontM  float64 `yaml:"font_medium"`
	FontS  float64 `yaml:"font_small"`
}

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
	return fmt.Sprintf("%.1f", m.Value)
}

func (m *Measurement) FormatAge() string {
	if m.Empty || m.AgeInMins < 3 {
		return ""
	}
	if m.AgeInMins < 30 {
		return fmt.Sprintf(">%dm", m.AgeInMins)
	} else {
		return ">30m"
	}
}

func (m *Measurement) FormatSlope() string {
	if m.Empty || m.Slope == 0 {
		return ""
	}
	if m.Slope > 0 {
		return "↑"
	} else if m.Slope < 0 {
		return "↓"
	}
	return ""
}

type Weather struct {
	Icon          string
	Precipitation float64
	Hourly        []Forecast
}

type Forecast struct {
	Icon          string
	Dt            time.Time
	Temp          float64
	FeelsLike     float64
	Precipitation float64
	Pop           int
}

func loadEnv() {
	envPath := os.Getenv("CONFIG")
	log.Debug().Msgf("Reading environment from %s", envPath)
	var err error
	envFile, err = godotenv.Read(path.Join(envPath, ".env"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}

	log.Debug().Msgf("Read environment %v", envFile)
}

func loadGenerateImageConfigurations(files [][]byte, output string) ([]*GenerateImageConfiguration, error) {
	configSlice := []*GenerateImageConfiguration{}
	for _, file := range files {
		config := &GenerateImageConfiguration{}
		err := yaml.Unmarshal(file, config)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to unmarshal file")
			return nil, err
		}
		config.Output = filepath.Join(output, config.Output)
		configSlice = append(configSlice, config)
	}
	return configSlice, nil

}

func main() {
	output := os.Getenv("OUTPUT")

	loadEnv()

	c1, err := configuration.ReadFile("configuration/800x600.yaml")
	if err != nil {
		log.Error().Err(err).Msg("Failed to read 800x600.yaml")
		panic(1)
	}
	c2, err := configuration.ReadFile("configuration/1024x758.yaml")
	if err != nil {
		log.Error().Err(err).Msg("Failed to read 1024x758.yaml")
		panic(1)
	}
	imgConfigFiles := [][]byte{
		c1,
		c2,
	}
	imageConfigurations, err := loadGenerateImageConfigurations(imgConfigFiles, output)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read image configurations")
		return
	}
	measurements := readData()

	weather, err := getWeather()
	if err != nil {
		// log but don't panic
		log.Error().Err(err).Msg("Failed to read weather")
	}

	for _, imageConfig := range imageConfigurations {
		drawResult(measurements, weather, imageConfig)
	}
}

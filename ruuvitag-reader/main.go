package main

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
	"github.com/peterhellberg/ruuvitag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

const CONFIG_PATH = "config.yml"

var (
	configuration = map[string]string{}
	envFile       = map[string]string{}
	handledMacs   = []string{}
	macs          = []string{}
	devices       = map[string]int{}
)

type Measurement struct {
	MAC                       string  `json:"mac"`
	Temperature               float64 `json:"temp"`
	Humidity                  float64 `json:"humidity"`
	Pressure                  uint32  `json:"pressure"`
	AccelerationX             int16   `json:"accelerationX"`
	AccelerationY             int16   `json:"accelerationY"`
	AccelerationZ             int16   `json:"accelerationZ"`
	Battery                   uint16  `json:"battery"`
	TxPower                   int8    `json:"txPower"`
	MovementCounter           uint8   `json:"movementCounter"`
	MeasurementSequenceNumber uint16  `json:"measurementSequenceNumber"`
	Rssi                      int     `json:"rssi"`
}

func loadConfiguration() {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	file, err := os.ReadFile(path.Join(exPath, "config.yml"))
	if err != nil {
		log.Error().Err(err).Msgf("Failed to read %s", CONFIG_PATH)
		panic(err)
	}

	err = yaml.Unmarshal(file, &configuration)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to unmarshal configuration")
		panic(err)
	}
	log.Info().Msgf("Loaded configuration: %v", configuration)

	for k := range configuration {
		macs = append(macs, strings.ToUpper(strings.ReplaceAll(k, "_", ":")))
	}

	envFile, err = godotenv.Read(path.Join(exPath, ".env"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}

	log.Debug().Msgf("Read environment %v", envFile)
}

func setup(ctx context.Context) context.Context {
	d, err := linux.NewDevice()
	if err != nil {
		panic(err)
	}
	ble.SetDefaultDevice(d)

	return ble.WithSigHandler(context.WithCancel(ctx))
}

func main() {
	log.Info().Msg("Loading configuration...")
	loadConfiguration()

	log.Info().Msg("Setting up...")
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), 1*time.Minute))
	ctx = setup(ctx)

	log.Info().Msg("Scanning...")
	ble.Scan(ctx, false, handler, filter)
}

func handler(a ble.Advertisement) {
	log.Debug().Msgf("Handling %s", a.LocalName())

	deviceKey := strings.ToUpper(strings.ReplaceAll(a.Addr().String(), ":", "_"))

	label, ok := configuration[deviceKey]

	if !ok {
		log.Warn().Msgf("Got device with addr %s and converted device key %s that does not exist in configuration", a.Addr().String(), deviceKey)
		return
	}

	if ruuvitag.IsRAWv1(a.ManufacturerData()) {
		raw, err := ruuvitag.ParseRAWv1(a.ManufacturerData())
		if err != nil {
			log.Error().Err(err).Msgf("Failed to parse v1 data from device %s", a.Addr())
			return
		}
		handle(raw.Temperature, raw.Humidity, raw.Pressure, raw.Acceleration.X, raw.Acceleration.Y, raw.Acceleration.Z, raw.Battery, 0, 0., 0, a.RSSI(), a.Addr().String(), label)
	} else if ruuvitag.IsRAWv2(a.ManufacturerData()) {
		raw, err := ruuvitag.ParseRAWv2(a.ManufacturerData())
		if err != nil {
			log.Error().Err(err).Msgf("Failed to parse v2 data from device %s", a.Addr())
			return
		}
		log.Debug().Msgf("[%s] %s, RSSI: %3d: %+v\n", a.Addr(), label, a.RSSI(), raw)
		handle(raw.Temperature, raw.Humidity, raw.Pressure, raw.Acceleration.X, raw.Acceleration.Y, raw.Acceleration.Z, raw.Battery, raw.TXPower, raw.Movement, raw.Sequence, a.RSSI(), a.Addr().String(), label)
	} else {
		log.Error().Msgf("Got an advertisement that did not belong to any known Ruuvitag %s", a.Addr())
		return
	}
}

func filter(a ble.Advertisement) bool {
	return slices.Contains(macs, strings.ToUpper(a.Addr().String()))
}

func handle(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string, label string) {

	if slices.Contains(handledMacs, mac) {
		log.Debug().Msgf("Device with mac %s already handled this round", mac)
		return
	}

	err := writeToInfluxDB(t, h, p, ax, ay, az, b, tx, mv, seq, rssi, mac, label)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to write data to Influxdb from device %s", mac)
	} else {
		log.Info().Msgf("Successfully wrote data to Influxdb from device %s", mac)
		handledMacs = append(handledMacs, mac)
	}

	err = sendToRuuviHttp(t, h, p, ax, ay, az, b, tx, mv, seq, rssi, mac)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to send data to Ruuvi HTTP from device %s", mac)
	} else {
		log.Info().Msgf("Successfully sent data to Ruuvi HTTP from device %s", mac)
	}
}

func writeToInfluxDB(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string, label string) error {

	log.Debug().Msg("Writing to Influxdb...")

	url := envFile["INFLUXDB_URL"]
	token := envFile["INFLUXDB_TOKEN"]
	client := influxdb2.NewClient(url, token)

	org := envFile["INFLUXDB_ORG"]
	bucket := envFile["INFLUXDB_BUCKET"]
	writeAPI := client.WriteAPIBlocking(org, bucket)
	tags := map[string]string{
		"mac":       mac,
		"tag_label": label,
	}
	fields := map[string]interface{}{
		"temperature":               t,
		"humidity":                  h,
		"pressure":                  p,
		"accelerationX":             ax,
		"accelerationY":             ay,
		"accelerationZ":             az,
		"batteryVoltage":            b,
		"txPower":                   tx,
		"movementCounter":           mv,
		"measurementSequenceNumber": seq,
		"rssi":                      rssi,
	}
	point := write.NewPoint("ruuvi_measurements", tags, fields, time.Now())

	err := writeAPI.WritePoint(context.Background(), point)
	return err
}

func sendToRuuviHttp(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string) error {

	m := Measurement{
		MAC:                       mac,
		Temperature:               t,
		Humidity:                  h,
		Pressure:                  p,
		AccelerationX:             ax,
		AccelerationY:             ay,
		AccelerationZ:             az,
		Battery:                   b,
		TxPower:                   tx,
		MovementCounter:           mv,
		MeasurementSequenceNumber: seq,
		Rssi:                      rssi,
	}
	url := envFile["RUUVI_HTTP_SERVER_ADD_MEASUREMENT_API_URL"]

	var client = resty.New().SetLogger(newLogger(&log.Logger))
	r := client.R()

	r.SetHeader("Content-Type", "application/json")
	r.SetBody(m)

	resp, err := r.Post(url)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to add measurement %v", m)
		return err
	}
	if resp.IsError() {
		log.Error().Msgf("Got %d as response code", resp.StatusCode())
		return err
	}
	return nil

}

type restyZeroLogger struct {
	logger *zerolog.Logger
}

func newLogger(zlogger *zerolog.Logger) *restyZeroLogger {
	return &restyZeroLogger{
		logger: zlogger,
	}
}
func (l *restyZeroLogger) Errorf(format string, v ...interface{}) {
	l.logger.Error().Msgf(format, v...)
}

func (l *restyZeroLogger) Warnf(format string, v ...interface{}) {
	l.logger.Warn().Msgf(format, v...)
}

func (l *restyZeroLogger) Debugf(format string, v ...interface{}) {
	l.logger.Debug().Msgf(format, v...)
}

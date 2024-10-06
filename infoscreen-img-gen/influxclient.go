package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/rs/zerolog/log"
)

func readData() []Measurement {

	log.Debug().Msg("Reading data from Influxdb...")

	helEuTz, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		log.Error().Err(err).Msgf("Could not load Europe/Helsinki location")
		panic(err)
	}

	url := envFile["INFLUXDB_URL"]
	token := envFile["INFLUXDB_TOKEN"]
	client := influxdb2.NewClient(url, token)

	org := envFile["INFLUXDB_ORG"]
	bucket := envFile["INFLUXDB_BUCKET"]

	query := fmt.Sprintf(`from(bucket: "%s")
			|> range(start: -30m)
            |> filter(fn: (r) => r._measurement == "ruuvi_measurements")
            |> filter(fn: (r) => r._field == "temperature")
            |> group(columns: ["mac"])
			|> sort(columns: ["_time"], desc: true)
			|> limit(n: 5)`,
		bucket)

	results, err := client.QueryAPI(org).Query(context.Background(), query)
	log.Debug().Msg("Queried results...")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query data")
		return []Measurement{}
	}
	log.Debug().Msg("No errors encountered...")
	rawMeasurements := []RawMeasurement{}
	for results.Next() {
		mac := results.Record().ValueByKey("mac").(string)
		label := results.Record().ValueByKey("tag_label").(string)
		val := float32(results.Record().Value().(float64))
		at := results.Record().Time().In(helEuTz)
		log.Info().Msgf("Label: %s, mac: %s, value: %f, at: %s", label, mac, val, at)

		rawMeasurement := RawMeasurement{
			Label: label,
			Mac:   mac,
			Value: val,
			At:    at,
		}
		rawMeasurements = append(rawMeasurements, rawMeasurement)
	}
	if err := results.Err(); err != nil {
		log.Fatal().Err(err)
	}

	measurements := processRawMeasurements(rawMeasurements)
	for _, m := range measurements {
		log.Debug().Msgf("Label: %s, value: %f, slope: %f, age in mins: %d", m.Label, m.Value, m.Slope, m.AgeInMins)
	}

	return measurements
}

func processRawMeasurements(rawMeasurements []RawMeasurement) []Measurement {

	measurements := []Measurement{}

	groupedByMac := map[string][]RawMeasurement{}
	for _, r := range rawMeasurements {
		vals, exists := groupedByMac[r.Mac]
		if !exists {
			vals = []RawMeasurement{}
		}
		vals = append(vals, r)
		groupedByMac[r.Mac] = vals
	}

	for _, v := range groupedByMac {
		sort.Slice(v, func(i, j int) bool {
			return v[i].At.Before(v[j].At)
		})
		a := v[0]
		b := v[len(v)-1]
		dt := b.Value - a.Value
		dx := float32(b.At.Sub(a.At).Minutes())
		slope := dt / dx

		now := time.Now()
		ageInMins := int(now.Sub(b.At).Minutes())

		measurement := Measurement{
			Label:     getLabel(b.Mac),
			Value:     b.Value,
			Slope:     slope,
			AgeInMins: ageInMins,
		}
		measurements = append(measurements, measurement)
	}

	sensorsInOrder := strings.Split(envFile["SENSORS_IN_ORDER"], ";")

	sort.Slice(measurements, func(i, j int) bool {
		return slices.Index(sensorsInOrder, measurements[i].Label) < slices.Index(sensorsInOrder, measurements[j].Label)
	})

	allMeasurements := []Measurement{}
	for _, m := range sensorsInOrder {
		var matchingMeasurement Measurement
		match := false
		for _, mm := range measurements {
			if mm.Label == m {
				matchingMeasurement = mm
				match = true
				break
			}
		}
		if !match {
			matchingMeasurement = Measurement{
				Label: m,
				Empty: true,
			}
		}
		allMeasurements = append(allMeasurements, matchingMeasurement)
	}

	return allMeasurements
}

func getLabel(mac string) string {
	return envFile[strings.ToUpper(strings.ReplaceAll(mac, ":", "_"))]
}

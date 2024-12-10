package main

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

func readData() []Measurement {

	log.Debug().Msg("Reading data from Postgres...")

	helEuTz, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		log.Error().Err(err).Msgf("Could not load Europe/Helsinki location")
		panic(err)
	}

	connUrl := envFile["POSTGRESQL_CONN_URL"]
	conn, err := pgx.Connect(context.Background(), connUrl)
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to database")
		return []Measurement{}
	}
	defer conn.Close(context.Background())

	sql := `WITH RankedMeasurements AS (
    SELECT d.mac as mac, d.label as label, m.temperature as temperature, m.created_at as created_at, ROW_NUMBER() OVER (PARTITION BY d.mac ORDER BY m.created_at DESC) AS rn
    FROM measurement m
    LEFT JOIN device d ON d.id = m.device_id
    WHERE m.created_at >= NOW() - INTERVAL '30 minutes'
)

SELECT mac, label, temperature, created_at
FROM RankedMeasurements
WHERE rn <= 5
ORDER BY mac, created_at DESC;
`
	rows, err := conn.Query(context.Background(), sql)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query data")
		return []Measurement{}
	}
	log.Debug().Msg("No errors encountered...")
	rawMeasurements := []RawMeasurement{}
	for rows.Next() {
		var (
			mac         string
			label       string
			temperature float64
			at          time.Time
		)
		rows.Scan(&mac, &label, &temperature, &at)

		at = at.In(helEuTz)
		rawMeasurement := RawMeasurement{
			Label: label,
			Mac:   mac,
			Value: float32(temperature),
			At:    at,
		}

		log.Debug().Msgf("Read measurement: %v", rawMeasurement)

		rawMeasurements = append(rawMeasurements, rawMeasurement)
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

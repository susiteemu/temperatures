# Temperatures mono repo

This repository contains all the moving parts of my newer temperature measurement and info display system.

The moving parts are:
- Ruuvitag Bluetooth Reader: reads data from Ruuvitags via Bluetooth and writes them to Influxdb
- Ruuvitag Bluetooth Minireader: a ESP32 microcontroller based Bluetooth reader that sends data via WLAN to a endpoint
- Ruuvitag HTTP Server: has a endpoint to add measurements

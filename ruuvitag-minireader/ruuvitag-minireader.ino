/*
  Modified from https://github.com/tspycher/ArduinoRuuviClient/blob/main/RuuviClient/RuuviClient.ino
*/

#include <ArduinoBLE.h>
#include <WiFi.h>
#include <ArduinoJson.h>
#include <HTTPClient.h>

const char* serverName = "http://192.168.1.207/ruuvitag/measurements";

//------------------------------------------
//WIFI
const char* ssid = "<ssid>";
const char* pw = "<password>";
//------------------------------------------


float temperature, humidity;
uint32_t pressure;
uint16_t battery;
String device;

bool new_data = false;
bool debug = false;
bool writeToDb = true;

void blePeripheralDiscoveredHandler(BLEDevice central) {
  if(central.hasManufacturerData() && central.hasAdvertisementData()) {
    // Check PDF From: https://www.bluetooth.com/specifications/assigned-numbers/
    // 0x0499 Ruuvi Innovations Ltd.
    // Decimal: 4 153
    uint8_t manufacturer[central.manufacturerDataLength()];
    central.manufacturerData(manufacturer, central.manufacturerDataLength());
    
    if((int)manufacturer[0] == 153 && (int)manufacturer[1] == 4) {
      Serial.print("Discovered event from RuuviTag with Mac: ");
      Serial.println(central.address());
    } else {
      if (debug) {
        Serial.print("Advertisement is from an unknown Device with Manufacturer Id: ");
        Serial.print((int)manufacturer[1], HEX);
        Serial.print((int)manufacturer[0], HEX);
        Serial.println();
      }
      return;
    }
  } else {
    // no manufacturer Data available
    if(debug) {
      Serial.print("Discovered Device with mac: ");
      Serial.print(central.address());
      Serial.println(" but does no advertised data or manufacturer data");
    }
    return;
  }

  // Load the Advertised Data and store the values
  int l = central.advertisementDataLength();
  uint8_t value[l];
  central.advertisementData(value, l);

  int l_payload = (int)value[3];
  int format = (int)value[7];
  int payload_start = 8;

  if (format == 5) { // Data Format 5 Protocol Specification (RAWv2)
    // https://github.com/ruuvi/ruuvi-sensor-protocols/blob/master/dataformat_05.md

    int16_t raw_temperature = (int16_t)value[payload_start]<<8 | (int16_t)value[payload_start+1];
    int16_t raw_humidity = (int16_t)value[payload_start+2]<<8 | (int16_t)value[payload_start+3];
    uint16_t raw_pressure = (uint16_t)value[payload_start+4]<<8 | (uint16_t)value[payload_start+5];
    uint16_t raw_battery = (uint16_t)value[payload_start+9]<<8 | (uint16_t)value[payload_start+10];
    raw_battery = raw_battery & 0x07FF; // mask to get only first 11-bits

    device = central.address();
    temperature = raw_temperature * 0.005;
    humidity = raw_humidity * 0.0025;
    pressure = raw_pressure + 50000;
    battery = raw_battery + 1600;
    new_data = true;
  } else {
    Serial.print("Unknown Data Format from RuuviTag received: ");
    Serial.println(format);
  }
}

void send_data() {
  if(!new_data)
    return;

  new_data = false;
  Serial.print("Device: ");
  Serial.print(device);
  Serial.print(" ");
  Serial.print("Temperature: ");
  Serial.print(temperature);
  Serial.print("C ");
  Serial.print("Humidity: ");
  Serial.print(humidity);
  Serial.print("% ");
  Serial.print("Pressure: ");
  Serial.print(pressure);
  Serial.print("Pa ");
  Serial.print("Battery: ");
  Serial.print(battery);
  Serial.print("mV ");
  Serial.println();

  if (!writeToDb)
    return;

  HTTPClient http;

  String serverPath = serverName;
  
  // Your Domain name with URL path or IP address with path
  http.begin(serverPath.c_str());
  
  http.addHeader("Content-Type", "application/json");

  DynamicJsonDocument doc(1024);
  doc["temp"] = temperature;
  doc["mac"] = device;
  doc["humidity"] = humidity;
  doc["pressure"] = pressure;
  doc["battery"] = battery;

  // Data to send with HTTP POST
  String httpRequestData = "";
  serializeJson(doc, httpRequestData);

  // Send HTTP POST request
  int httpResponseCode = http.POST(httpRequestData);
  
  if (httpResponseCode>0) {
    Serial.print("HTTP Response code: ");
    Serial.println(httpResponseCode);
  }
  else {
    Serial.print("Error code: ");
    Serial.println(httpResponseCode);
  }
  // Free resources
  http.end();
}

void setup() {
  Serial.begin(9600);
  while (!Serial);

  //Setup WIFI
  WiFi.begin(ssid, pw);
  Serial.println("");

  //Wait for WIFI connection
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  Serial.println("");
  Serial.print("Connected to ");
  Serial.println(ssid);
  Serial.print("IP address: ");
  Serial.println(WiFi.localIP());

  esp_sleep_enable_timer_wakeup(120 * 1000000); // deep sleep for 2 minutes

  if (!BLE.begin()) {
    Serial.println("starting Bluetooth® Low Energy module failed!");
    while (1);
  }
  Serial.println("Ruuvi Prototype to listen for Ruuvi Advertisements and Decode data");
  Serial.println("------------------------------------------------------------------");
  BLE.setEventHandler(BLEDiscovered, blePeripheralDiscoveredHandler);
  BLE.scan();
  delay(1000);
  Serial.println("Starting to poll...");
  for (int i = 0; i < 60; i++) {
    Serial.print("Poll round: ");
    Serial.println(i);
    BLE.poll();
    send_data();
    delay(1000);
  }
  Serial.println("Stopped polling");
  BLE.stopScan();
  delay(1000);
  BLE.end();
  delay(1000);
  
  
  Serial.println("It is time to sleep");
  Serial.flush();
  esp_deep_sleep_start();
}

void loop() {
}

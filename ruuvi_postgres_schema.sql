
CREATE TABLE device (
  device SERIAL PRIMARY KEY,
  mac VARCHAR(48) NOT NULL,
  label VARCHAR NOT NULL,
);


CREATE TABLE measurement (
    id SERIAL PRIMARY KEY,
    device_id INT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    temperature NUMERIC(5,2),
    humidity NUMERIC(5,2),
	pressure INTEGER,
	acceleration_x INTEGER,
	acceleration_y INTEGER,
	acceleration_z INTEGER,
	battery_voltage INTEGER,
	tx_power INTEGER,
	movement_counter BIGINT,
	measurement_sequence_number BIGINT,
	rssi INTEGER,
  CONSTRAINT fk_device
  	FOREIGN KEY(device_id)
  	REFERENCES device(id)
);


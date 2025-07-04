//
// Code generated by go-jet DO NOT EDIT.
//
// WARNING: Changes to this file may cause incorrect behavior
// and will be lost if the code is regenerated
//

package table

import (
	"github.com/go-jet/jet/v2/postgres"
)

var Device = newDeviceTable("public", "device", "")

type deviceTable struct {
	postgres.Table

	// Columns
	ID    postgres.ColumnInteger
	Mac   postgres.ColumnString
	Label postgres.ColumnString

	AllColumns     postgres.ColumnList
	MutableColumns postgres.ColumnList
	DefaultColumns postgres.ColumnList
}

type DeviceTable struct {
	deviceTable

	EXCLUDED deviceTable
}

// AS creates new DeviceTable with assigned alias
func (a DeviceTable) AS(alias string) *DeviceTable {
	return newDeviceTable(a.SchemaName(), a.TableName(), alias)
}

// Schema creates new DeviceTable with assigned schema name
func (a DeviceTable) FromSchema(schemaName string) *DeviceTable {
	return newDeviceTable(schemaName, a.TableName(), a.Alias())
}

// WithPrefix creates new DeviceTable with assigned table prefix
func (a DeviceTable) WithPrefix(prefix string) *DeviceTable {
	return newDeviceTable(a.SchemaName(), prefix+a.TableName(), a.TableName())
}

// WithSuffix creates new DeviceTable with assigned table suffix
func (a DeviceTable) WithSuffix(suffix string) *DeviceTable {
	return newDeviceTable(a.SchemaName(), a.TableName()+suffix, a.TableName())
}

func newDeviceTable(schemaName, tableName, alias string) *DeviceTable {
	return &DeviceTable{
		deviceTable: newDeviceTableImpl(schemaName, tableName, alias),
		EXCLUDED:    newDeviceTableImpl("", "excluded", ""),
	}
}

func newDeviceTableImpl(schemaName, tableName, alias string) deviceTable {
	var (
		IDColumn       = postgres.IntegerColumn("id")
		MacColumn      = postgres.StringColumn("mac")
		LabelColumn    = postgres.StringColumn("label")
		allColumns     = postgres.ColumnList{IDColumn, MacColumn, LabelColumn}
		mutableColumns = postgres.ColumnList{MacColumn, LabelColumn}
		defaultColumns = postgres.ColumnList{IDColumn}
	)

	return deviceTable{
		Table: postgres.NewTable(schemaName, tableName, alias, allColumns...),

		//Columns
		ID:    IDColumn,
		Mac:   MacColumn,
		Label: LabelColumn,

		AllColumns:     allColumns,
		MutableColumns: mutableColumns,
		DefaultColumns: defaultColumns,
	}
}

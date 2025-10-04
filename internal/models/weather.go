package models

type Weather struct {
	Temperature string `db:"temperature" json:"temperature"`
}
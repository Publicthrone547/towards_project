package models

type Crime struct {
	SafetyAssessment string `db:"safety_assessment" json:"safety_assessment"`
}
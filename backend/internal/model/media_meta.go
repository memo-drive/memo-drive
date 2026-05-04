package model

import "time"

type MediaMeta struct {
	Width     int        `json:"width,omitempty"`
	Height    int        `json:"height,omitempty"`
	Duration  float64    `json:"duration,omitempty"`
	TakenAt   *time.Time `json:"taken_at,omitempty"`
	Latitude  *float64   `json:"latitude,omitempty"`
	Longitude *float64   `json:"longitude,omitempty"`
	Camera    string     `json:"camera,omitempty"`
	Codec     string     `json:"codec,omitempty"`
	Bitrate   int        `json:"bitrate,omitempty"`
	Format    string     `json:"format,omitempty"`
}

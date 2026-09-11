package model

type Place struct {
	OSMType     string
	OSMID       int64
	Name        string
	Normalized  string
	HouseNumber string
	Street      string
	Postcode    string
	City        string
	District    string
	Country     string
	CountryCode string
	PlaceType   string
	Lat         float64
	Lon         float64
	Population  *int64
	SearchText  string
}

type Result struct {
	ID          int64   `json:"osm_id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Type        string  `json:"type"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DistanceM   float64 `json:"distance_m"`
	HouseNumber string  `json:"housenumber"`
	Street      string  `json:"street"`
	Postcode    string  `json:"postcode"`
	City        string  `json:"city"`
	District    string  `json:"district"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
}

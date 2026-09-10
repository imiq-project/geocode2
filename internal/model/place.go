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
	ID          int64   `json:"id"`
	Name        string  `json:"name,omitempty"`
	Type        string  `json:"type"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DistanceM   float64 `json:"distance_m,omitempty"`
	HouseNumber string  `json:"house_number,omitempty"`
	Street      string  `json:"street,omitempty"`
	Postcode    string  `json:"postcode,omitempty"`
	City        string  `json:"city,omitempty"`
	District    string  `json:"district,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
}

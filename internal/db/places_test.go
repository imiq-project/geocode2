package db

import (
	"context"
	"os"
	"testing"
)

func TestDeletePlacesRemovesAllRows(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("GEOCODER_DATABASE_URL")
	if url == "" {
		url = "postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable"
	}

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `TRUNCATE places`); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO places (
			osm_type, osm_id, name, normalized_name, house_number, street,
			postcode, city, district, country, country_code, place_type,
			geom, importance, population, search_text
		)
		VALUES (
			'N', 1, 'Test Place', 'test place', '', '', '', '', '', '', '', 'place',
			ST_SetSRID(ST_MakePoint(13.4, 52.1), 4326), 1.0, NULL, 'test place'
		)`); err != nil {
		t.Fatal(err)
	}

	if err := DeletePlaces(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM places`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected all places to be deleted, got %d rows", count)
	}
}

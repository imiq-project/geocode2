# Geocoder

Go + PostgreSQL/PostGIS + pgvector OSM PBF geocoder.

## Build

```bash
go mod tidy
go build ./...
```

## Database

```bash
docker compose up -d postgres
go run ./cmd/migrate
```

## Import

```bash
osmium add-locations-to-ways magdeburg.osm.pbf -o magdeburg_ways.osm.pbf 
go run ./cmd/importer \
  --pbf /data/region.osm.pbf \
  --database 'postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable' \
  --workers 8 \
  --batch-size 10000
```

The PBF is streamed from disk. It is not loaded into memory.

## API

```bash
go run ./cmd/geocoder \
  --database 'postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable'
```

```bash
curl 'http://localhost:8080/v1/search?q=Alexanderplatz&limit=10'
curl 'http://localhost:8080/v1/search?q=Alexanderplatz&bbox=13.37,52.50,13.45,52.55'
curl 'http://localhost:8080/v1/search?q=Alexanderplatz&lat=52.5219&lon=13.4132&radius=5000'
curl 'http://localhost:8080/v1/reverse?lat=52.5219&lon=13.4132'
curl 'http://localhost:8080/v1/autocomplete?q=alexan&limit=10'
```

## Important

This version fixes the dependency and PBF scanner issues in the first draft.

For a full production geocoder, the next major step is adding proper OSM way/relation geometry, boundary hierarchy, house-number interpolation, multilingual names, and a real embedding backfill worker.

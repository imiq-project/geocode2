package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"example.com/geocoder/internal/db"
	"example.com/geocoder/internal/embeddings"
	"example.com/geocoder/internal/model"
	"example.com/geocoder/internal/normalize"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

func main() {
	pbf := flag.String("pbf", "", "path to .osm.pbf")
	database := flag.String("database", "postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable", "database URL")
	workers := flag.Int("workers", runtime.NumCPU(), "decoder workers")
	batchSize := flag.Int("batch-size", 10000, "database batch size")
	embeddingsURL := flag.String("embeddings-url", "http://ollama:11434/v1/embeddings", "api endpoint for generating embeddings")
	flag.Parse()

	if *pbf == "" {
		log.Fatal("--pbf is required")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, *database)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	f, err := os.Open(*pbf)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	err = db.Create(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("migration complete")

	scanner := osmpbf.New(ctx, f, *workers)
	defer scanner.Close()

	batch := make([]model.Place, 0, *batchSize)
	var count int64

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := db.CopyPlaces(ctx, pool, batch); err != nil {
			log.Fatal(err)
		}
		count += int64(len(batch))
		log.Printf("imported %d places", count)
		batch = batch[:0]
	}

	for scanner.Scan() {
		if p, ok := placeFromOSM(scanner.Object()); ok {
			batch = append(batch, p)
			if len(batch) >= *batchSize {
				flush()
			}
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("finished: %d places", count)

	placeTypes, err := db.GetAllPlaceTypes(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%d place types", len(placeTypes))

	placeTypeEmbeddings, err := embeddings.GenerateEmbeddings(placeTypes, *embeddingsURL)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("embeddings generated")
	embeddingsByPlaceType := make(map[string][]float64)
	for idx, placeType := range placeTypes {
		embeddingsByPlaceType[placeType] = placeTypeEmbeddings[idx]
	}
	err = db.UpdateEmbedding(ctx, pool, embeddingsByPlaceType)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("embeddings inserted")
}

func tagsFromObject(obj osm.Object) osm.Tags {
	switch o := obj.(type) {
	case *osm.Node:
		return o.Tags
	case *osm.Way:
		return o.Tags
	case *osm.Relation:
		return o.Tags
	default:
		return nil
	}
}

func placeFromOSM(obj osm.Object) (model.Place, bool) {
	tags := tagsFromObject(obj)
	name := strings.TrimSpace(tags.Find("name"))
	if name == "" {
		return model.Place{}, false
	}

	lat, lon, ok := point(obj)
	if !ok {
		return model.Place{}, false
	}

	ptype := tags.Find("place")
	if ptype == "" {
		for _, key := range []string{"amenity", "shop", "tourism", "railway", "highway"} {
			if ptype = tags.Find(key); ptype != "" {
				ptype = ptype + " " + key
				break
			}
		}
	}

	if ptype == "" {
		if tags.Find("addr:housenumber") == "" && tags.Find("addr:street") == "" {
			return model.Place{}, false
		}
	}

	var population *int64
	if s := tags.Find("population"); s != "" {
		if n, err := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64); err == nil {
			population = &n
		}
	}

	p := model.Place{
		OSMType:     string(obj.ObjectID().Type()),
		OSMID:       int64(obj.ObjectID().Ref()),
		Name:        name,
		Normalized:  normalize.Text(name),
		HouseNumber: tags.Find("addr:housenumber"),
		Street:      tags.Find("addr:street"),
		Postcode:    tags.Find("addr:postcode"),
		City:        first(tags.Find("addr:city"), tags.Find("addr:town"), tags.Find("addr:village")),
		District:    tags.Find("addr:suburb"),
		Country:     tags.Find("addr:country"),
		CountryCode: tags.Find("addr:country"),
		PlaceType:   ptype,
		Lat:         lat,
		Lon:         lon,
		Population:  population,
	}
	p.SearchText = normalize.SearchText(
		p.Name, p.PlaceType, p.HouseNumber, p.Street, p.Postcode,
		p.City, p.District, p.Country,
	)
	return p, true
}

func first(a, b, c string) string {
	if a != "" {
		return a
	}
	if b != "" {
		return b
	}
	return c
}

func point(obj osm.Object) (float64, float64, bool) {
	switch v := obj.(type) {
	case *osm.Node:
		return v.Lat, v.Lon, true
	case *osm.Way:
		if len(v.Nodes) == 0 {
			return 0, 0, false
		}
		var lat, lon float64
		for _, n := range v.Nodes {
			if n.Lat == 0 && n.Lon == 0 {
				continue
			}
			lat += n.Lat
			lon += n.Lon
		}
		n := float64(len(v.Nodes))
		return lat / n, lon / n, true
	case *osm.Relation:
		return 0, 0, false
	default:
		fmt.Printf("Unhandled type: %T\n", obj)
		return 0, 0, false
	}
}

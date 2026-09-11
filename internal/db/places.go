package db

import (
	"context"
	"fmt"
	"strings"

	"example.com/geocoder/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

func CopyPlaces(ctx context.Context, pool *pgxpool.Pool, places []model.Place) error {
	if len(places) == 0 {
		return nil
	}

	// COPY goes through a staging table so geometry can be built by PostgreSQL.
	_, err := pool.Exec(ctx, `
        CREATE TEMP TABLE IF NOT EXISTS places_stage (
            osm_type text, osm_id bigint, name text, normalized_name text,
            house_number text, street text, postcode text, city text, district text,
            country text, country_code text, place_type text,
            lat double precision, lon double precision,
            population bigint, search_text text
        )`) // TODO: ON COMMIT DROP
	if err != nil {
		return err
	}

	rows := make([][]any, 0, len(places))
	for _, p := range places {
		rows = append(rows, []any{
			p.OSMType, p.OSMID, p.Name, p.Normalized,
			p.HouseNumber, p.Street, p.Postcode, p.City, p.District,
			p.Country, p.CountryCode, p.PlaceType,
			p.Lat, p.Lon, p.Population, p.SearchText,
		})
	}

	_, err = pool.CopyFrom(ctx, pgx.Identifier{"places_stage"},
		[]string{
			"osm_type", "osm_id", "name", "normalized_name",
			"house_number", "street", "postcode", "city", "district",
			"country", "country_code", "place_type", "lat", "lon",
			"population", "search_text",
		},
		pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
        INSERT INTO places (
            osm_type, osm_id, name, normalized_name, house_number, street,
            postcode, city, district, country, country_code, place_type,
            geom, population, search_text
        )
        SELECT
            osm_type, osm_id, name, normalized_name, house_number, street,
            postcode, city, district, country, country_code, place_type,
            ST_SetSRID(ST_MakePoint(lon, lat), 4326),
        	population, search_text
        FROM places_stage
        ON CONFLICT (osm_type, osm_id) DO UPDATE SET
            name = EXCLUDED.name,
            normalized_name = EXCLUDED.normalized_name,
            house_number = EXCLUDED.house_number,
            street = EXCLUDED.street,
            postcode = EXCLUDED.postcode,
            city = EXCLUDED.city,
            district = EXCLUDED.district,
            country = EXCLUDED.country,
            country_code = EXCLUDED.country_code,
            place_type = EXCLUDED.place_type,
            geom = EXCLUDED.geom,
            population = EXCLUDED.population,
            search_text = EXCLUDED.search_text`)
	return err
}

func Search(
	ctx context.Context,
	pool *pgxpool.Pool,
	q string,
	queryEmbedding []float64,
	bbox *[4]float64,
	lat, lon *float64,
	radius float64,
	limit int,
) ([]model.Result, error) {
	if limit <= 0 {
		return nil, nil
	}

	var results []model.Result
	vectorThreshold := .2

	// ---------------------------------------------------------------
	// 1. Vector search
	// ---------------------------------------------------------------

	if len(queryEmbedding) > 0 {
		// pgvector-go's Vector type is float32, which matches the
		// vector(384) column.
		v := make([]float32, len(queryEmbedding))
		for i, x := range queryEmbedding {
			v[i] = float32(x)
		}

		embedding := pgvector.NewVector(v)

		args := []any{embedding}
		where := []string{
			"embedding IS NOT NULL",
			"embedding <=> $1 < $2",
		}
		args = append(args, vectorThreshold)

		n := 3

		// Bounding box.
		if bbox != nil {
			where = append(where, fmt.Sprintf(
				"geom && ST_MakeEnvelope($%d,$%d,$%d,$%d,4326)",
				n, n+1, n+2, n+3,
			))
			args = append(args,
				bbox[0],
				bbox[1],
				bbox[2],
				bbox[3],
			)
			n += 4
		}

		// Radius.
		if lat != nil && lon != nil && radius > 0 {
			where = append(where, fmt.Sprintf(
				"ST_DWithin("+
					"geom::geography, "+
					"ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography, "+
					"$%d)",
				n,
				n+1,
				n+2,
			))
			args = append(args, *lon, *lat, radius)
			n += 3
		}

		// Geographic distance is only for the result field.
		distanceSQL := "0::double precision"

		if lat != nil && lon != nil {
			args = append(args, *lon, *lat)

			distanceSQL = fmt.Sprintf(
				"ST_Distance("+
					"geom::geography, "+
					"ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography)",
				n,
				n+1,
			)
			n += 2
		}

		args = append(args, limit)

		sql := fmt.Sprintf(`
			SELECT
				osm_id,
				name,
				place_type,
				ST_Y(geom),
				ST_X(geom),
				%s AS distance,
				house_number,
				street,
				postcode,
				city,
				district,
				country,
				country_code
			FROM places
			WHERE %s
			ORDER BY embedding <=> $1
			LIMIT $%d
		`,
			distanceSQL,
			strings.Join(where, " AND "),
			n,
		)

		rows, err := pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, fmt.Errorf("vector search: %w", err)
		}

		for rows.Next() {
			var r model.Result

			if err := rows.Scan(
				&r.ID,
				&r.Name,
				&r.Type,
				&r.Lat,
				&r.Lon,
				&r.DistanceM,
				&r.HouseNumber,
				&r.Street,
				&r.Postcode,
				&r.City,
				&r.District,
				&r.Country,
				&r.CountryCode,
			); err != nil {
				rows.Close()
				return nil, err
			}

			results = append(results, r)
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}

		rows.Close()
	}

	// If enough vector matches passed the threshold, we're done.
	if len(results) >= limit {
		return results[:limit], nil
	}

	// ---------------------------------------------------------------
	// 2. Full-text / trigram fallback
	// ---------------------------------------------------------------

	remaining := limit - len(results)

	args := []any{q}
	where := []string{
		"(normalized_name % $1 OR " +
			"to_tsvector('simple', search_text) @@ " +
			"plainto_tsquery('simple', $1))",
	}

	n := 2

	// Bounding box.
	if bbox != nil {
		where = append(where, fmt.Sprintf(
			"geom && ST_MakeEnvelope($%d,$%d,$%d,$%d,4326)",
			n,
			n+1,
			n+2,
			n+3,
		))
		args = append(args,
			bbox[0],
			bbox[1],
			bbox[2],
			bbox[3],
		)
		n += 4
	}

	// Radius.
	if lat != nil && lon != nil && radius > 0 {
		where = append(where, fmt.Sprintf(
			"ST_DWithin("+
				"geom::geography, "+
				"ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography, "+
				"$%d)",
			n,
			n+1,
			n+2,
		))
		args = append(args, *lon, *lat, radius)
		n += 3
	}

	// Exclude vector results.
	if len(results) > 0 {
		placeholders := make([]string, len(results))

		for i, r := range results {
			placeholders[i] = fmt.Sprintf("$%d", n)
			args = append(args, r.ID)
			n++
		}

		where = append(
			where,
			fmt.Sprintf(
				"id NOT IN (%s)",
				strings.Join(placeholders, ","),
			),
		)
	}

	// Geographic distance.
	distanceSQL := "0::double precision"

	if lat != nil && lon != nil {
		args = append(args, *lon, *lat)

		distanceSQL = fmt.Sprintf(
			"ST_Distance("+
				"geom::geography, "+
				"ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography)",
			n,
			n+1,
		)
		n += 2
	}

	args = append(args, remaining)

	sql := fmt.Sprintf(`
		SELECT
			osm_id,
			name,
			place_type,
			ST_Y(geom),
			ST_X(geom),
			%s AS distance,
			house_number,
			street,
			postcode,
			city,
			district,
			country,
			country_code
		FROM places
		WHERE %s
		ORDER BY similarity(search_text, $1) DESC, distance
		LIMIT $%d
	`,
		distanceSQL,
		strings.Join(where, " AND "),
		n,
	)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("full-text search: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r model.Result

		if err := rows.Scan(
			&r.ID,
			&r.Name,
			&r.Type,
			&r.Lat,
			&r.Lon,
			&r.DistanceM,
			&r.HouseNumber,
			&r.Street,
			&r.Postcode,
			&r.City,
			&r.District,
			&r.Country,
			&r.CountryCode,
		); err != nil {
			return nil, err
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func SimpleSearch(ctx context.Context, pool *pgxpool.Pool, q string, queryEmbedding []float64, bbox *[4]float64, lat, lon *float64, radius float64, limit int) ([]model.Result, error) {
	args := []any{q}
	where := []string{"(normalized_name % $1 OR to_tsvector('simple', search_text) @@ plainto_tsquery('simple', $1))"}
	n := 2

	if bbox != nil {
		where = append(where, fmt.Sprintf(
			"geom && ST_MakeEnvelope($%d,$%d,$%d,$%d,4326)", n, n+1, n+2, n+3))
		args = append(args, bbox[0], bbox[1], bbox[2], bbox[3])
		n += 4
	}

	distanceSQL := "0::double precision"
	if lat != nil && lon != nil {
		args = append(args, *lon, *lat)
		distanceSQL = fmt.Sprintf(
			"ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography)",
			n, n+1)
		n += 2
		if radius > 0 {
			args = append(args, radius)
			where = append(where, fmt.Sprintf(
				"ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography,$%d)",
				n-2, n-1, n))
			n++
		}
	}

	args = append(args, limit)
	sql := fmt.Sprintf(`
		SELECT id,name,place_type,ST_Y(geom),ST_X(geom),%s AS distance,
			house_number,street,postcode,city,district,country,country_code
		FROM places
		WHERE %s
		ORDER BY similarity(search_text,$1) DESC, distance
		LIMIT $%d`,
		distanceSQL,
		strings.Join(where, " AND "),
		n,
	)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Result
	for rows.Next() {
		var r model.Result
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Lat, &r.Lon, &r.DistanceM,
			&r.HouseNumber, &r.Street, &r.Postcode, &r.City, &r.District,
			&r.Country, &r.CountryCode); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func Reverse(ctx context.Context, pool *pgxpool.Pool, lat, lon, radius float64) (*model.Result, error) {
	row := pool.QueryRow(ctx, `
        SELECT id,name,place_type,ST_Y(geom),ST_X(geom),
               ST_Distance(geom::geography, p::geography),
               house_number,street,postcode,city,district,country,country_code
        FROM places
        CROSS JOIN LATERAL ST_SetSRID(ST_MakePoint($1,$2),4326) p
        WHERE ST_DWithin(geom::geography,p::geography,$3)
        ORDER BY geom <-> p
        LIMIT 1`, lon, lat, radius)

	var r model.Result
	err := row.Scan(&r.ID, &r.Name, &r.Type, &r.Lat, &r.Lon, &r.DistanceM,
		&r.HouseNumber, &r.Street, &r.Postcode, &r.City, &r.District,
		&r.Country, &r.CountryCode)
	return &r, err
}

func Autocomplete(ctx context.Context, pool *pgxpool.Pool, q string, limit int) ([]model.Result, error) {
	rows, err := pool.Query(ctx, `
        SELECT id,name,place_type,ST_Y(geom),ST_X(geom),
               house_number,street,postcode,city,district,country,country_code
        FROM places
        WHERE normalized_name LIKE $1 || '%'
           OR normalized_name % $1
        ORDER BY
            CASE WHEN normalized_name LIKE $1 || '%' THEN 0 ELSE 1 END,
            similarity(normalized_name,$1) DESC
        LIMIT $2`, strings.ToLower(strings.TrimSpace(q)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Result
	for rows.Next() {
		var r model.Result
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Lat, &r.Lon,
			&r.HouseNumber, &r.Street, &r.Postcode, &r.City, &r.District,
			&r.Country, &r.CountryCode); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func GetAllPlaceTypes(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT place_type
		FROM places
		WHERE place_type IS NOT NULL
		  AND place_type <> ''
		ORDER BY place_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var placeType string
		if err := rows.Scan(&placeType); err != nil {
			return nil, err
		}
		out = append(out, placeType)
	}

	return out, rows.Err()
}

func UpdateEmbedding(ctx context.Context, pool *pgxpool.Pool, placeTypeEmbeddings map[string][]float64) error {
	if len(placeTypeEmbeddings) == 0 {
		return nil
	}

	for placeType, embedding := range placeTypeEmbeddings {
		if len(embedding) == 0 {
			continue
		}

		values := make([]float32, len(embedding))
		for i, v := range embedding {
			values[i] = float32(v)
		}

		_, err := pool.Exec(ctx, `
			UPDATE places
			SET embedding = $1
			WHERE place_type = $2
		`, pgvector.NewVector(values), placeType)
		if err != nil {
			return err
		}
	}
	return nil
}

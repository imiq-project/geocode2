CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE places (
    id              BIGSERIAL PRIMARY KEY,
    osm_type        TEXT NOT NULL,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    normalized_name TEXT NOT NULL,
    house_number    TEXT,
    street          TEXT,
    postcode        TEXT,
    city            TEXT,
    district        TEXT,
    country         TEXT,
    country_code    TEXT,
    place_type      TEXT NOT NULL,
    geom            geometry(Point, 4326) NOT NULL,
    embedding       vector(384),
    population      BIGINT,
    search_text     TEXT NOT NULL,
    UNIQUE (osm_type, osm_id)
);

CREATE INDEX IF NOT EXISTS places_geom_gist
    ON places USING GIST (geom);
CREATE INDEX IF NOT EXISTS places_name_trgm
    ON places USING GIN (normalized_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS places_search_fts
    ON places USING GIN (to_tsvector('simple', search_text));
CREATE INDEX IF NOT EXISTS places_embedding_hnsw
    ON places USING hnsw (embedding vector_cosine_ops);

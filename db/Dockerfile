FROM pgvector/pgvector:pg17

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       postgis \
       postgresql-17-postgis-3 \
    && apt-get clean

# Colossus
Over-engineered thumbnail app.


# Setup
## Prerequisites
1. Installed `docker` with `docker-compose`.
2. About 1 GB RAM machine :)

## Prepare configs
```sh
cp example_configs/.env example_configs/* ./

# Not pretty
for path_raw in $(cat .env | awk -F= '{{ print $2 }}')
do
  path=$(eval echo "$path_raw")
  echo Creating dir "$path"
  mkdir -p "$path" -v
  chmod -R 777 "$path"
done;
```
Change something if you feel that way.

## Run
```sh
docker-compose up -d --build
```

## Try
1. Upload image
   ```sh
   curl -L -X POST 'http://localhost:10001/upload-image' \
     -F 'file=@"/path/to/image"'
   ```
   In response, you will see the UUID of your image.
2. Retrieve image
   ```sh
   # Raw
   curl -L -X GET 'http://localhost:10001/retrieve-image/raw/1719c1fa-4e31-4191-969c-3843de8a2463-raw.jpg'
   
   # Processed
   curl -L -X GET 'http://localhost:10001/retrieve-image/processed/1719c1fa-4e31-4191-969c-3843de8a2463-processed.jpg'
   ```
3. Visit UIs
   * Grafana: http://localhost:10106/dashboards
   * Kowl: http://localhost:10104/
   * Minio: http://localhost:10102/

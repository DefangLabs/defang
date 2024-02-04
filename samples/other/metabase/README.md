# Deploy Metabase with Defang

This is a simple example of how to deploy Metabase with Defang.

Note that this is a very simple example and should not be used in production. If you want to run a basic production setup, you need a database (e.g. PostgreSQL). You can set the `MB_DB_CONNECTION_URI` secret using `defang secret set --name MB_DB_CONNECTION_URI` and uncomment the `secrets` section of the `docker-compose.yml` file.
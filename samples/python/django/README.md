This is a simple example of how to run Django on Defang. It is a simple Todo app that uses SQLite as the database (so data is *not* persisted between deployments). We will be putting together an example with a managed database soon.

The app includes a management command which is run on startup to create a superuser with the username `admin` and password `admin`. This means you can login to the admin interface at `/admin/` and see the Django admin interface without any additional steps. The `example_app` is already registered and the `Todo` model is already set up to be managed in the admin interface.

The Dockerfile and compose files are already set up for you and are ready to be deployed. Serving is done using [Gunicorn](https://gunicorn.org/) and uses [WhiteNoise](https://whitenoise.readthedocs.io/en/latest/) for static files. The `CSRF_TRUSTED_ORIGINS` setting is configured to allow the app to run on a `defang.dev` subdomain.

## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a> to describe the basic image of your applications.
2. A <a href="https://docs.defang.io/docs/concepts/compose">docker-compose file</a> to define and run multi-container Docker applications.
3. A <a href="https://docs.docker.com/build/building/context/#dockerignore-files">.dockerignore</a> file to comply with the size limit (10MB).

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>
Plus, make sure that you have properly set your environment variables like `AWS_PROFILE`, `AWS_REGION`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY`.

## A Step-by-Step Guide
1. (optional) If you are using Defang BYOC, make sure to update the `CSRF_TRUSTED_ORIGINS` setting in the `settings.py` file to include an appropriate domain.
2. Open the terminal and type `defang login`
3. Type `defang compose up` in the CLI
4. Now your application will be launched

# Use an official Python runtime as a parent image
FROM python:3.10.4-slim-buster

# Set environment variables
ENV PYTHONDONTWRITEBYTECODE 1
ENV PYTHONUNBUFFERED 1

# Set work directory
WORKDIR /code

# Install dependencies
COPY requirements.txt /code/
RUN pip install --no-cache-dir -r requirements.txt

# Copy project
COPY . /code/

# Collect static files
RUN python manage.py collectstatic --noinput

# Start server
CMD python manage.py migrate && python manage.py createsuperauto && gunicorn defang_sample.wsgi:application --bind 0.0.0.0:8000
# Google Cloud Run Deployment

Quick start documentation:
https://cloud.google.com/run/docs/quickstarts/build-and-deploy

## Really quick start

Install GCP SDK:

    brew cask install google-cloud-sdk

Set default project:

    gcloud config set project passages-signup

Build and push container image:

    gcloud builds submit --tag gcr.io/passages-signup/passages-signup

Deploy to Cloud Run:

    gcloud run deploy --image gcr.io/passages-signup/passages-signup --platform managed

And same for a particular service:

    gcloud run deploy --image gcr.io/passages-signup/passages-signup --platform managed nanoglyph-signup
    gcloud run deploy --image gcr.io/passages-signup/passages-signup --platform managed passages-signup
# Kubernetes setup

## Docker image build

Make sure a Docker image is built:

    docker build -t brandur/passages-signup .

(Without a tag like `brandur/passages-signup:tag` specified, this will build
`brandur/passages-signup:latest`.)

Make sure Docker Hub login is active:

    docker login

Push the image:

    docker push brandur/passages-signup

## Kubernetes configuration

Make sure a Kubernetes configuration file is available (`$KUBECONFIG` should be
set and the file it's pointing to should exist).

    kubectl apply -f kubernetes/
    kubectl apply -f kubernetes/passages-signup.yaml

### Digital Ocean

Install `doctl`:

    brew install doctl
    brew auth init

Creating a certificate that automatically renews with Let's Encrypt:

    doctl compute certificate create --name "passages-signup-k8s-brandur-org" --dns-names "passages-signup.k8s.brandur.org" --type lets_encrypt

Creating secrets:

    kubectl create secret generic database-url --from-literal=DATABASE_URL=postgres://localhost/passages-signup
    kubectl create secret generic mailgun-api-key --from-literal=MAILGUN_API_KEY=key-xxx

<!--
# vim: set tw=79:
-->

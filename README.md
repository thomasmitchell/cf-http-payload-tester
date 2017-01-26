# cf-http-payload-tester

This app can be `curl`ed on it's `/match/{route}` endpoint to send an http
payload to another instance of this application. The body of the request that
gets sent is configurable.

## Usage

### Building the project

Make sure you have Go 1+ installed and your $GOPATH is configured, and then 
`go get github.com/thomasmmitchell/cf-http-payload-tester`. This should build
a version of the project and put the resulting binary in your `$GOPATH/bin`
folder, but if it doesn't, you can navigate to 
`$GOPATH/src/github.com/thomasmmitchell/cf-http-payload-tester` and run 
`go build`.

### Launching the server

The binary that gets created will probably be called `cf-http-payload-tester`.

```bash
PORT=1234 ./cf-http-payload-tester -t 8s
```

Set the `PORT` environment variable to the port number you want the server to
listen on (Cloud Foundry does this for you when you push an app).

The payload is configured by creating the `test_payload` file in the same
directory as the binary and having its contents be what you want to send as the
body of the HTTP request.

The app can be configured for different timeouts, payload file targets, and to 
send out an https request for checks. See `--help` for more details.


### Making requests

The application listens on the `/check/{route}` endpoint for GET requests, where 
{route} is the
hostname of the server you'd like to check the request against. {route} actually
gets substituted into the sent URL like `http://{route}/listen`, so form your
request accordingly. This will send the configured payload to the route you've
specified.

```bash
curl localhost:1234/check/localhost:1234
```

The app also listen on the `/pull` endpoint for get requests. This will send the
configured payload back as a response to that request.

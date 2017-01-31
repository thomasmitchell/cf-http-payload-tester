package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"bufio"

	"io"

	"github.com/gorilla/mux"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	defaultPayloadFilename = "test_payload"
	defaultTimeout         = "5s"
)

var (
	payloadFile       *os.File
	payloadFileLength int64
	outgoingClient    = &http.Client{}
	protocol          = "http"
	//Need a non-const version of http.StatusBadRequest
	badRequestCode = int(http.StatusBadRequest)

	//COMMAND LINE STUFF
	cmdline         = kingpin.New("cf-http-payload-tester", "Test your HTTP requests on Cloud Foundry")
	timeout         = cmdline.Flag("timeout", "Time in seconds to wait for response to check calls").Short('t').Default(defaultTimeout).Duration()
	useHTTPS        = cmdline.Flag("https-out", "Use https in outbound URL instead of http").Short('s').Bool()
	payloadFilename = cmdline.Flag("payload", "Target payload file").Short('p').Default(defaultPayloadFilename).String()
)

func main() {
	cmdline.HelpFlag.Short('h')
	kingpin.MustParse(cmdline.Parse(os.Args[1:]))

	err := setup()
	if err != nil {
		log.Fatal(err.Error())
	}

	err = launchAPIServer()
	log.Fatal(err.Error())
}

func setup() (err error) {
	//Get the file to send over HTTP
	payloadFile, err = os.Open(fmt.Sprintf("%s", *payloadFilename))
	if err != nil {
		return fmt.Errorf("Could not open payload file: %s", err.Error())
	}

	//Get length of file for reporting
	stats, err := payloadFile.Stat()
	if err != nil {
		return fmt.Errorf("error stat-ing file: %s", err.Error())
	}

	payloadFileLength = stats.Size()

	//Make sure the PORT env var is set
	if os.Getenv("PORT") == "" {
		return fmt.Errorf("Please set PORT environment variable with port for server to listen on")
	}

	//Make sure that PORT is numeric
	_, err = strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return fmt.Errorf("PORT environment variable was not numeric")
	}

	log.Printf("Setting HTTP client timeout to %s", *timeout)
	outgoingClient.Timeout = *timeout

	if *useHTTPS {
		protocol = "https"
	}
	log.Printf("Setting protocol to %s", protocol)

	return nil
}

func launchAPIServer() error {
	router := mux.NewRouter()
	router.HandleFunc("/check/{route}", checkHandler).Methods("GET")
	router.HandleFunc("/gencheck/{route}/{bytes}", generatedCheckHandler).Methods("GET")
	router.HandleFunc("/listen", listenHandler).Methods("POST")
	router.HandleFunc("/pull", pullHandler).Methods("GET")

	return http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), router)
}

type responseJSON struct {
	Status       *int   `json:"status,omitempty"`
	ErrorMessage string `json:"error,omitempty"`
	Bytes        *int64 `json:"bytes"`
}

func responsify(r *responseJSON) []byte {
	ret, err := json.Marshal(r)
	if err != nil {
		panic("Couldn't marshal JSON")
	}
	return ret
}

func checkHelper(w http.ResponseWriter, r *http.Request, sourceBody io.Reader, numBytes int64) {
	outgoingResp := &responseJSON{Bytes: &numBytes}
	route := mux.Vars(r)["route"]
	//Make our outgoing POST request to send to the other app
	outgoingRequest, err := http.NewRequest("POST", fmt.Sprintf("%s://%s/listen", protocol, route), sourceBody)
	if err != nil {
		panic("Could not form http request")
	}
	//Set the Content-Type and if X-Payload-Tracer is given, put that in too
	outgoingRequest.Header.Set("Content-Type", "text/plain")
	outgoingRequest.Header.Set("X-Payload-Tracer", r.Header.Get("X-Payload-Tracer"))
	w.Header().Set("X-Payload-Tracer", r.Header.Get("X-Payload-Tracer"))

	resp, err := outgoingClient.Do(outgoingRequest)

	//Reset payload file seek position to the start of the file
	defer func() {
		_, err = payloadFile.Seek(0, io.SeekStart)
		if err != nil {
			panic("Could not reset payload file seek position")
		}
	}()

	if err != nil {
		outgoingResp.ErrorMessage = fmt.Sprintf("Error while sending request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(responsify(outgoingResp))
		return
	}

	if resp.StatusCode == 500 {
		outgoingResp.ErrorMessage = fmt.Sprintf("Remote server failed while reading request body")
	}

	w.WriteHeader(resp.StatusCode)
	outgoingResp.Status = &resp.StatusCode
	w.Write(responsify(outgoingResp))
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	checkHelper(w, r, bufio.NewReader(payloadFile), payloadFileLength)
}

func generatedCheckHandler(w http.ResponseWriter, r *http.Request) {
	numBytes := mux.Vars(r)["bytes"]
	numBytesInt, err := strconv.ParseInt(numBytes, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(responsify(&responseJSON{Status: &badRequestCode, ErrorMessage: "Could not parse body size in request URL"}))
	}

	if numBytesInt < 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(responsify(&responseJSON{Status: &badRequestCode, ErrorMessage: "Cannot send negative amount of bytes"}))
	}
	checkHelper(w, r, io.LimitReader(rand.New(rand.NewSource(time.Now().UnixNano())), numBytesInt), numBytesInt)
}

func listenHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Payload-Tracer", r.Header.Get("X-Payload-Tracer"))
	_, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

func pullHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Payload-Tracer", r.Header.Get("X-Payload-Tracer"))
	var err error
	//Reset payload file seek position to the start of the file
	defer func() {
		_, err = payloadFile.Seek(0, io.SeekStart)
		if err != nil {
			panic("Could not reset payload file seek position")
		}
	}()

	//Gotta take the file in in chunks so that we don't blow up the RAM if
	// somebody tests with a huge file.
	const bufferSize = 8 * 1024 //8KiB please
	buffer := make([]byte, bufferSize)
	var bytesRead = bufferSize //Initial value to kick off the while loop
	for bytesRead == bufferSize && err != io.EOF {
		bytesRead, err = payloadFile.Read(buffer)
		w.Write(buffer[:bytesRead])
	}
}

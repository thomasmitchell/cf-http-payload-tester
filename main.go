package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"bufio"

	"github.com/gorilla/mux"
)

const (
	payloadFilename = "test_payload"
	defaultTimeout  = 5
)

var (
	payloadFile       *os.File
	payloadFileLength int64
	outgoingClient    = &http.Client{Timeout: defaultTimeout * time.Second}
)

func main() {
	err := setup()
	if err != nil {
		log.Fatal(err.Error())
	}

	err = launchAPIServer()
	log.Fatal(err.Error())
}

func setup() (err error) {
	//Get the file to send over HTTP
	payloadFile, err = os.Open(fmt.Sprintf("./%s", payloadFilename))
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

	//Set http timeout to custom value if specified.
	if len(os.Args) > 1 {
		//Make sure its numeric
		customTimeout, err := strconv.ParseInt(os.Args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("Timeout argument not numeric")
		}

		log.Printf("Setting HTTP client timeout to %d seconds", customTimeout)

		outgoingClient.Timeout = time.Duration(customTimeout) * time.Second
	}

	return nil
}

func launchAPIServer() error {
	router := mux.NewRouter()
	router.HandleFunc("/check/{route}", checkHandler).Methods("GET")
	router.HandleFunc("/listen", listenHandler).Methods("POST")

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

func checkHandler(w http.ResponseWriter, r *http.Request) {
	outgoingResp := &responseJSON{Bytes: &payloadFileLength}

	route := mux.Vars(r)["route"]
	resp, err := outgoingClient.Post(fmt.Sprintf("http://%s/listen", route), "text/plain", bufio.NewReader(payloadFile))

	if err != nil {
		outgoingResp.ErrorMessage = fmt.Sprintf("Error while sending request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(responsify(outgoingResp))
		return
	}

	//Not sure this can even happen... but...
	if resp.StatusCode != 200 {
		outgoingResp.ErrorMessage = fmt.Sprintf("Non 200-code returned from request to listening server: %d", resp.StatusCode)
	}

	w.WriteHeader(resp.StatusCode)
	outgoingResp.Status = &resp.StatusCode
	w.Write(responsify(outgoingResp))
}

func listenHandler(w http.ResponseWriter, r *http.Request) {
	//I mean... TCP guarantees that if we're this far, the body is correct
	// So.... if we got this far, the payload was successfully sent
	w.WriteHeader(http.StatusOK)
}

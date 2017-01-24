package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"bufio"

	"github.com/gorilla/mux"
)

const payloadFilename = "test_payload"

var (
	payloadFile       *os.File
	payloadFileLength int64
)

func main() {
	err := setup()
	if err != nil {
		log.Fatalf(err.Error())
	}

	apiErrChan := make(chan error, 0)
	go launchAPIServer(apiErrChan)
	select {
	case err = <-apiErrChan:
		log.Fatalf(err.Error())
	}
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

	return nil
}

func launchAPIServer(errChan chan<- error) {
	router := mux.NewRouter()
	router.HandleFunc("/check/{route}", checkHandler).Methods("GET")
	router.HandleFunc("/listen", listenHandler).Methods("POST")

	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), router)
}

type responseJSON struct {
	Status       *int   `json:"status,omitempty"`
	ErrorMessage string `json:"error,omitempty"`
	Bytes        *int64 `json:"bytes"`
}

func responsify(r responseJSON) []byte {
	ret, err := json.Marshal(&r)
	if err != nil {
		panic("Couldn't marshal JSON")
	}
	return ret
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	outgoingResp := responseJSON{Bytes: &payloadFileLength}
	defer w.Write(responsify(outgoingResp))

	route := mux.Vars(r)["route"]
	resp, err := http.Post(route, "text/plain", bufio.NewReader(payloadFile))

	if err != nil {
		outgoingResp.ErrorMessage = fmt.Sprintf("Error while sending request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	//Not sure this can even happen... but...
	if resp.StatusCode != 200 {
		outgoingResp.ErrorMessage = fmt.Sprintf("Non 200-code returned from request to listening server: %d", resp.StatusCode)
	}

	w.WriteHeader(resp.StatusCode)
	outgoingResp.Status = &resp.StatusCode
}

func listenHandler(w http.ResponseWriter, r *http.Request) {
	//I mean... TCP guarantees that if we're this far, the body is correct
	// So.... if we got this far, the payload was successfully sent
	w.WriteHeader(http.StatusOK)
}

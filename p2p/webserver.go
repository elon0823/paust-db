package p2p 

import (
	"net/http"
	"github.com/gorilla/mux"
	"encoding/json"
	BC "github.com/elon0823/paust-db/blockchain"
	"time"
	"log"
	"io"
)
type Message struct {
	BPM int32
}

type WebServer struct {
	Chain *BC.Blockchain
	P2PManager *P2PManager
	Address string
	Port string
	Mux http.Handler
	ReadTimeout time.Duration
	WriteTimeout time.Duration
	MaxHeaderBytes int
}

func NewWebServer(bchain *BC.Blockchain, p2pManager *P2PManager, address string, port string, timeout time.Duration, maxHeaderBytes int) (*WebServer, error) {

	webserver := WebServer{
		Chain: bchain,
		P2PManager: p2pManager,
		Address: address,
		Port: port, 
		Mux: mux.NewRouter(),
		ReadTimeout: timeout,
		WriteTimeout: timeout,
		MaxHeaderBytes: maxHeaderBytes,
	}
	webserver.Mux = webserver.makeMuxRouter()

	return &webserver, nil
}
func (webserver *WebServer) Run() error {
	
	//httpPort := os.Getenv("PORT")
	log.Println("HTTP Server Listening on port :", webserver.Port)
	s := &http.Server{
		Addr:           ":" + webserver.Port,
		Handler:        webserver.Mux,
		ReadTimeout:    webserver.ReadTimeout,
		WriteTimeout:   webserver.WriteTimeout,
		MaxHeaderBytes: webserver.MaxHeaderBytes,
	}

	if err := s.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

func (webserver *WebServer) makeMuxRouter() http.Handler {
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", webserver.handleGetBlockchain).Methods("GET")
	muxRouter.HandleFunc("/", webserver.handleWriteBlock).Methods("POST")
	return muxRouter
}

func (webserver *WebServer) handleGetBlockchain(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.MarshalIndent(webserver.Chain.GetChain(), "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(bytes))
}

// takes JSON payload as an input for heart rate (BPM)
func (webserver *WebServer) handleWriteBlock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var msg Message

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&msg); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	mutex.Lock()
	webserver.Chain.AddBlock(msg.BPM)

	mutex.Unlock()

	webserver.P2PManager.BroadcastChain("")
	respondWithJSON(w, r, http.StatusCreated, webserver.Chain.LastBlock())
}

func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	response, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}
	w.WriteHeader(code)
	w.Write(response)
}
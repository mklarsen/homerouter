package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

type api struct {
	cfg     Config
	svc     *Service
	backend Backend
	token   [32]byte
}

func (a *api) register(mux *http.ServeMux) {
	a.token = sha256.Sum256([]byte(a.cfg.APIToken))
	mux.Handle("GET /api/v1/server", a.auth(http.HandlerFunc(a.server)))
	mux.Handle("GET /api/v1/peers", a.auth(http.HandlerFunc(a.listPeers)))
	mux.Handle("POST /api/v1/peers", a.auth(http.HandlerFunc(a.createPeer)))
	mux.Handle("DELETE /api/v1/peers/{id}", a.auth(http.HandlerFunc(a.deletePeer)))
}

func (a *api) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sum := sha256.Sum256([]byte(got))
		// Hashing first keeps the comparison length-independent.
		if !ok || subtle.ConstantTimeCompare(sum[:], a.token[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="vpn"`)
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *api) server(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"publicKey":  a.backend.PublicKey(),
		"endpoint":   a.cfg.Endpoint,
		"subnet":     a.cfg.Pool.Prefix().String(),
		"allowedIps": a.cfg.AllowedIPs,
	})
}

func (a *api) listPeers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.List())
}

type createPeerRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
}

type createPeerResponse struct {
	Peer Peer `json:"peer"`
	// Config is a ready-to-use client file. It contains the private key only
	// when the server generated the key pair; that key is not stored.
	Config string `json:"config"`
	// QRSVG encodes Config for scanning with a phone; same secrecy as Config.
	QRSVG string `json:"qrSvg,omitempty"`
}

func (a *api) createPeer(w http.ResponseWriter, r *http.Request) {
	a.create(w, r, CreateOptions{})
}

func (a *api) create(w http.ResponseWriter, r *http.Request, opt CreateOptions) {
	var req createPeerRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON with name and optional publicKey")
		return
	}

	peer, priv, err := a.svc.Create(r.Context(), req.Name, req.PublicKey, opt)
	if err != nil {
		a.fail(w, err)
		return
	}
	cfgText := a.clientConfig(peer, priv)
	qrSVG, err := qrSVG(cfgText)
	if err != nil {
		log.Printf("qr: %v", err) // the text config is still usable
	}
	writeJSON(w, http.StatusCreated, createPeerResponse{Peer: peer, Config: cfgText, QRSVG: qrSVG})
}

func (a *api) deletePeer(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (a *api) clientConfig(p Peer, priv string) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	if priv != "" {
		b.WriteString("PrivateKey = " + priv + "\n")
	} else {
		b.WriteString("PrivateKey = <your private key>\n")
	}
	b.WriteString("Address = " + p.Address + "/32\n")
	if a.cfg.DNS != "" {
		b.WriteString("DNS = " + a.cfg.DNS + "\n")
	}
	b.WriteString("\n[Peer]\n")
	b.WriteString("PublicKey = " + a.backend.PublicKey() + "\n")
	b.WriteString("Endpoint = " + a.cfg.Endpoint + "\n")
	b.WriteString("AllowedIPs = " + strings.Join(a.cfg.AllowedIPs, ", ") + "\n")
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

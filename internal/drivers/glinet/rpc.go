package glinet

// JSON-RPC conversation with the router: login (challenge, crypt, hash) and module calls.

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ahmadarif-lab/router-monitoring/internal/apperr"
	"github.com/ahmadarif-lab/router-monitoring/internal/netutil"
	"github.com/ahmadarif-lab/router-monitoring/internal/unixcrypt"
)

var errNotGLiNet = apperr.New("not_glinet", "This does not look like a GL.iNet router (firmware 4.x)")

// flexNum decodes a JSON number or a numeric string (GL.iNet mixes both, e.g. "total_rx").
type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexNum(v)
	return nil
}

// ---------- JSON-RPC ----------

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

var errAccessDenied = fmt.Errorf("access denied")

// rpc sends one JSON-RPC request. Call with g.mu held.
func (g *driver) rpc(method string, params any) (json.RawMessage, error) {
	g.nextID++
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": g.nextID, "method": method, "params": params})
	req, err := http.NewRequest("POST", g.base+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", netutil.UserAgent)
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, netutil.Unreachable(g.base, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var r rpcResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, errNotGLiNet
	}
	if r.Error != nil {
		switch {
		case r.Error.Message == "Access denied":
			return nil, errAccessDenied
		case strings.Contains(r.Error.Message, "over limit"):
			var d struct {
				Wait int `json:"wait"`
			}
			_ = json.Unmarshal(r.Error.Data, &d)
			return nil, apperr.New("login_limit", fmt.Sprintf("Too many failed logins, wait %d seconds", d.Wait), "wait", strconv.Itoa(d.Wait))
		}
		return nil, apperr.New("api_error", "Router error: "+r.Error.Message, "detail", r.Error.Message)
	}
	var apiErr struct {
		Msg  string `json:"err_msg"`
		Code int    `json:"err_code"`
	}
	if json.Unmarshal(r.Result, &apiErr) == nil && (apiErr.Msg != "" || apiErr.Code != 0) {
		return nil, apperr.New("api_error", "Router error: "+apiErr.Msg, "detail", apiErr.Msg)
	}
	return r.Result, nil
}

func hashHex(method, s string) string {
	switch method {
	case "sha256":
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:])
	case "sha512":
		h := sha512.Sum512([]byte(s))
		return hex.EncodeToString(h[:])
	}
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// login performs challenge + login and stores the session id. Call with g.mu held.
func (g *driver) login() error {
	raw, err := g.rpc("challenge", map[string]string{"username": g.cr.User})
	if err != nil {
		if err == errAccessDenied {
			return apperr.ErrLoginRejected
		}
		return err
	}
	var c struct {
		Alg        int    `json:"alg"`
		Salt       string `json:"salt"`
		Nonce      string `json:"nonce"`
		HashMethod string `json:"hash-method"`
	}
	if json.Unmarshal(raw, &c) != nil || c.Nonce == "" || c.Salt == "" {
		return errNotGLiNet
	}
	cipher, err := unixcrypt.Crypt(g.cr.Pass, fmt.Sprintf("$%d$%s$", c.Alg, c.Salt))
	if err != nil {
		return errNotGLiNet
	}
	hash := hashHex(c.HashMethod, g.cr.User+":"+cipher+":"+c.Nonce)
	raw, err = g.rpc("login", map[string]string{"username": g.cr.User, "hash": hash})
	if err != nil {
		if err == errAccessDenied {
			return apperr.ErrLoginRejected
		}
		return err
	}
	var l struct {
		SID string `json:"sid"`
	}
	if json.Unmarshal(raw, &l) != nil || l.SID == "" {
		return apperr.ErrLoginRejected
	}
	g.sid = l.SID
	return nil
}

// call runs module.fn with the session, logging in first or again when the session is gone.
func (g *driver) call(module, fn string, params any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if g.sid == "" {
			if err := g.login(); err != nil {
				return err
			}
		}
		raw, err := g.rpc("call", []any{g.sid, module, fn, params})
		if err == errAccessDenied {
			g.sid = ""
			continue
		}
		if err != nil {
			return err
		}
		if out != nil {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
	return apperr.ErrLoginRejected
}

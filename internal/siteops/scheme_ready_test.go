package siteops

import (
	"errors"
	"net/http"
	"testing"
)

// After securing, nginx is serving the new scheme once the TLS endpoint answers
// at all, whatever the app returns: a 500 from the site is still proof the vhost
// is live, and waiting for 200 would hang on an app that is simply broken.
func TestSchemeServed(t *testing.T) {
	redirect := &http.Response{StatusCode: http.StatusMovedPermanently, Header: http.Header{"Location": []string{"https://x.test/"}}}

	cases := []struct {
		name    string
		resp    *http.Response
		err     error
		secured bool
		want    bool
	}{
		{"secured, TLS answers", &http.Response{StatusCode: 200}, nil, true, true},
		{"secured, app errors but vhost is live", &http.Response{StatusCode: 500}, nil, true, true},
		{"secured, connection refused", nil, errors.New("connection refused"), true, false},
		{"unsecured, plain vhost serves", &http.Response{StatusCode: 200}, nil, false, true},
		{"unsecured, still redirecting to https", redirect, nil, false, false},
		{"unsecured, connection refused", nil, errors.New("connection refused"), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemeServed(tc.resp, tc.err, tc.secured); got != tc.want {
				t.Errorf("schemeServed() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package progress

import (
	"net/http"
)

const contentSizeThreshold = 1024

type RoundTripper struct {
	inner http.RoundTripper
	pb    *DownloadBar
}

func NewRoundTripper(inner http.RoundTripper, pb *DownloadBar) *RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}

	rt := RoundTripper{
		inner: inner,
		pb:    pb,
	}

	return &rt
}

func (t *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.pb != nil && req.Body != nil && req.ContentLength >= contentSizeThreshold {
		t.pb.Init(req.ContentLength)
		req.Body = t.pb.bar.ProxyReader(req.Body)
	}
	resp, err := t.inner.RoundTrip(req)
	if t.pb != nil && resp != nil && resp.Body != nil && resp.ContentLength >= contentSizeThreshold {
		t.pb.Init(resp.ContentLength)
		resp.Body = t.pb.bar.ProxyReader(resp.Body)
	}
	return resp, err
}

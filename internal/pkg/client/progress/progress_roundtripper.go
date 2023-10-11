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
	dpb   DownloadProgressBar
}

func NewRoundTripper(inner http.RoundTripper, dpb DownloadProgressBar) *RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}

	rt := RoundTripper{
		inner: inner,
		dpb:   dpb,
	}

	return &rt
}

func (t *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if resp != nil && resp.Body != nil && resp.ContentLength >= contentSizeThreshold {
		t.dpb.Init(resp.ContentLength)
		resp.Body = t.dpb.bar.ProxyReader(resp.Body)
	}
	return resp, err
}

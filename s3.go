package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type s3Director struct {
	s3Svc  *s3.S3
	bucket string
	prefix string
}

func effectiveKey(prefix, userPath string) string {
	userPath = strings.Trim(userPath, "/")

	if prefix == "" {
		return userPath
	}

	if userPath == "" {
		return prefix
	}

	return prefix + "/" + userPath
}

func (s *s3Director) Direct(r *http.Request) {

	key := effectiveKey(s.prefix, r.URL.Path)

	var s3Req *request.Request
	switch r.Method {
	case http.MethodGet:
		s3Req, _ = s.s3Svc.GetObjectRequest(&s3.GetObjectInput{
			Bucket: &s.bucket,
			Key:    aws.String(key),
		})
	case http.MethodPut:
		s3Req, _ = s.s3Svc.PutObjectRequest(&s3.PutObjectInput{
			Bucket: &s.bucket,
			Key:    aws.String(key),
		})
	default:
		log.Fatal("Unsupported method: %s", r.Method)
	}

	purl, err := s3Req.Presign(10 * time.Minute)
	if err != nil {
		log.Fatal("Presign failed (%v) for request %+v", err, s3Req)
	}

	r.URL, _ = url.Parse(purl)
	r.Host = ""
}

func newS3Director(session *session.Session, url *url.URL) *s3Director {
	s3Svc := s3.New(session)

	bucket := url.Host
	prefix := strings.Trim(url.Path, "/")

	return &s3Director{
		s3Svc:  s3Svc,
		bucket: bucket,
		prefix: prefix,
	}
}

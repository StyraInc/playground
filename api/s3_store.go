package api

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3DataRequestStore is a DataRequestStore backed by s3.
type S3DataRequestStore struct {
	s3       *s3.S3
	bucket   string
	watchers map[string]update
	mu       sync.Mutex
}

// NewS3DataRequestStore creates new S3DataRequestStores.
func NewS3DataRequestStore(s3 *s3.S3, bucket string) *S3DataRequestStore {
	return &S3DataRequestStore{
		s3:       s3,
		bucket:   bucket,
		watchers: make(map[string]update),
	}
}

// Get a DataRequest (see api.DataRequestStore)
func (s *S3DataRequestStore) Get(key *StoreKey, _ *Principal) (DataRequest, bool, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.Id),
	}

	result, err := s.s3.GetObject(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == s3.ErrCodeNoSuchKey {
			return DataRequest{}, false, nil
		}
		return DataRequest{}, false, err
	}
	defer result.Body.Close()

	bs, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return DataRequest{}, true, err
	}

	var dr DataRequest
	if err := json.Unmarshal(bs, &dr); err != nil {
		return DataRequest{}, true, err
	}

	return dr, true, nil
}

// Put a DataRequest (see api.DataRequestStore)
func (s *S3DataRequestStore) Put(key *StoreKey, dr DataRequest, _ *Principal) (*StoreKey, error) {
	bs, err := json.Marshal(dr)
	if err != nil {
		return nil, err
	}

	input := &s3.PutObjectInput{
		Body:   aws.ReadSeekCloser(bytes.NewReader(bs)),
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key.Id),
	}

	_, err = s.s3.PutObject(input)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if up, ok := s.watchers[key.Id]; ok {
		up.cb(dr)
		delete(s.watchers, key.Id)
		close(up.done)
	}

	return key, nil
}

// Watch adds a watcher to provide change notifications when the store is changed
func (s *S3DataRequestStore) Watch(key *StoreKey, etag string, timeout time.Duration, cb func(DataRequest), principal *Principal) (bool, error) {
	dr, found, err := s.Get(key, principal)
	if err != nil {
		return false, err
	}

	if !found {
		return false, nil
	}

	if dr.Etag != etag {
		go cb(dr)
	} else {
		done := make(chan struct{})

		s.mu.Lock()
		s.watchers[key.Id] = update{
			cb:   cb,
			done: done,
		}
		s.mu.Unlock()
		go func() {
			select {
			case <-time.After(timeout):
				s.mu.Lock()
				defer s.mu.Unlock()
				if up, ok := s.watchers[key.Id]; ok {
					dr, _, _ = s.Get(key, principal)
					up.cb(dr)
					delete(s.watchers, key.Id)
				}
			case <-done:
				return
			}
		}()
	}

	return true, nil
}

// List the keys that are set with a given prefix (see api.DataRequestStore)
func (s *S3DataRequestStore) List(prefix *StoreKey, _ *Principal) ([]*StoreKey, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	}
	if prefix.Id != "" { // Dynamo has issues with empty strings, may not apply here but this won't hurt.
		input.Prefix = aws.String(prefix.Id)
	}

	keys := []*StoreKey{}
	for {
		res, err := s.s3.ListObjectsV2(input)
		if err != nil {
			return nil, err
		}

		for _, item := range res.Contents {
			keys = append(keys, &StoreKey{Id: *item.Key})
		}

		if !*res.IsTruncated {
			break
		}
		input.ContinuationToken = res.NextContinuationToken
	}

	return keys, nil
}

// ListAll the keys that are set (see api.DataRequestStore)
func (s *S3DataRequestStore) ListAll(principal *Principal) ([]*StoreKey, error) {
	return s.List(&StoreKey{Id: ""}, principal)
}

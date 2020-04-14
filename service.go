package main

import (
	"context"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Service encapsulates the needed configuration settings to make successful
// Gmail api calls
// The struct methods have not been optimized for concurrent use, create new
// instances for different goroutines
type Service struct {
	cnf    *jwt.Config
	UserID string
	srv    *gmail.Service
	// DefaultQ  is provided when filtering messages Gmail search box style
	DefaultQ        string
	WriterGenerator WriterGenerator
}

// NewService instantiates a new service struct for API calls
func NewService(config io.Reader, userID string) (*Service, error) {
	// Close reader if closable
	if closer, ok := config.(io.Closer); ok {
		defer closer.Close()
	}

	srv := &Service{
		UserID: userID,
	}

	// initialize the gmail service
	if err := srv.initializeJWTConfig(config); err != nil {
		return nil, err
	}

	// initialize the gmail service
	ctx := context.Background()
	gmailSrv, err := gmail.NewService(
		ctx, option.WithTokenSource(srv.cnf.TokenSource(ctx)))
	if err != nil {
		return nil, err
	}
	srv.srv = gmailSrv

	// Set default file generator
	srv.WriterGenerator = FileGenerator

	return srv, nil
}

func (srv *Service) initializeJWTConfig(r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	srv.cnf, err = google.JWTConfigFromJSON(
		data, gmail.GmailReadonlyScope, gmail.GmailModifyScope)
	if err != nil {
		return err
	}
	srv.cnf.Subject = srv.UserID
	return nil
}

// ListMessages fetches messages from the specified userID
func (srv *Service) ListMessages() ([]*gmail.Message, error) {
	call := srv.srv.Users.Messages.List(srv.UserID)
	if srv.DefaultQ != "" {
		call = call.Q(srv.DefaultQ)
	}
	rep, err := call.Do()
	if err != nil {
		return nil, err
	}

	return rep.Messages, nil
}

// WriterGenerator defines a function that defines where the attachment contents
// will be written to.
//
// This could be a byte.Buffer, os.File or any other interface that implements
// the writer interface
type WriterGenerator func(filename string) (io.Writer, error)

// ProcessedAttachment file contents read from the emails fetched
type ProcessedAttachment struct {
	Body     io.Reader
	Filename string
}

// ProcessedAttachments a slice of ProcessAttachment
type ProcessedAttachments []*ProcessedAttachment

// Close closes readers which also implement Closer interface
func (at ProcessedAttachments) Close() error {
	var err error

	for _, a := range at {
		if closer, ok := a.Body.(io.Closer); ok {
			if er := closer.Close(); er != nil && err == nil {
				err = er
			}
		}
	}

	return err
}

// FileGenerator returns a system file
func FileGenerator(filename string) (io.Writer, error) {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ProcessPDFAttachments reads pdf attachments from the emails fetched
func (srv *Service) ProcessPDFAttachments(fileGen WriterGenerator, markRead bool) (ProcessedAttachments, error) {
	msgs, err := srv.ListMessages()
	if err != nil {
		return nil, err
	}

	processedAttachments := make([]*ProcessedAttachment, 0)
	processedMsgs := make([]*gmail.Message, 0)
	// retrieve the payload part of the message
OUTER:
	for i, msg := range msgs {
		if msg, err := retrieveMessage(srv.srv, srv.UserID, msg.Id); err == nil {
			msgs[i] = msg
		}
		// Retrieve the parts with attachments
		parts, err := srv.retrieveMessageAttachments(msg, msg.Payload)
		if err != nil {
			continue
		}
		// Read the attachments to the provided writer from WriterGenerator
		for _, p := range parts {
			att, err := srv.processAttachment(msg, p)
			if err == nil {
				processedAttachments = append(processedAttachments, att)
			} else {
				// continue to the outer loop
				continue OUTER
			}
		}
		// add message to the list of processed messages
		processedMsgs = append(processedMsgs, msg)
	}

	// make the msgs are read if markRead is true
	if markRead {
		markAsRead(srv.srv, srv.UserID, processedMsgs)
	}

	return processedAttachments, nil
}

func (srv *Service) processAttachment(msg *gmail.Message, part *gmail.MessagePart) (*ProcessedAttachment, error) {
	filename := constructFilename(part, msg)
	f, err := srv.WriterGenerator(filename)
	if err != nil {
		return nil, err
	}

	fileContent, err := base64.URLEncoding.DecodeString(part.Body.Data)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(fileContent); err != nil {
		return nil, err
	}

	return &ProcessedAttachment{Filename: filename, Body: f.(io.Reader)}, nil
}

func (srv *Service) retrieveMessageAttachments(msg *gmail.Message, part *gmail.MessagePart) ([]*gmail.MessagePart, error) {
	if part.MimeType == "application/pdf" {
		// Retrieve the attachment
		body, err := retrieveAttachment(srv.srv, srv.UserID, msg, part.Body)
		if err != nil {
			return []*gmail.MessagePart{}, err
		}
		part.Body = body
		return []*gmail.MessagePart{part}, nil
	}

	parts := make([]*gmail.MessagePart, 0)
	for _, part := range part.Parts {
		prts, err := srv.retrieveMessageAttachments(msg, part)
		if err == nil {
			parts = append(parts, prts...)
		}
	}

	// Return an empty list
	return parts, nil
}

package gmail

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"
)

func processMessage(srv *gmail.Service, userID string, msg *gmail.Message) error {
	fmt.Printf("Message ID: %s\n", msg.Id)
	payload := msg.Payload
	if msg.Payload == nil {
		var err error
		if msg, err = retrieveMessage(srv, userID, msg.Id); err == nil {
			payload = msg.Payload
		}
	}

	if payload != nil {
		fmt.Printf("Snippet: %s\n", msg.Snippet)
		if err := processMessagePayload(srv, userID, msg, payload, 0); err != nil {
			return err
		}
	}

	return nil
}

func processMessagePayload(srv *gmail.Service, userID string, msg *gmail.Message, part *gmail.MessagePart, indent int) error {
	fmt.Printf("%sFile name: %s\n", strings.Repeat("-", indent), part.Filename)
	fmt.Printf("%sMime type: %s\n", strings.Repeat("-", indent), part.MimeType)
	if part.MimeType == "application/pdf" {
		if err := processPDFFile(srv, userID, part, msg); err != nil {
			log.Printf("Error process mpesa statement: %s\n", err)
			return err
		}
		return nil
	}

	for _, part := range part.Parts {
		if err := processMessagePayload(srv, userID, msg, part, indent+4); err != nil {
			return err
		}
	}

	return nil
}

func printPartHeaders(headers []*gmail.MessagePartHeader, indent int) {
	for _, header := range headers {
		fmt.Printf("%sName: %s\n", strings.Repeat("-", indent), header.Name)
		fmt.Printf("%sValue %s\n", strings.Repeat("-", indent), header.Value)
	}
}

func retrieveMessage(srv *gmail.Service, userID, msgID string) (*gmail.Message, error) {
	call := srv.Users.Messages.Get(userID, msgID)
	return call.Do()
}

func constructFilename(part *gmail.MessagePart, msg *gmail.Message) string {
	return fmt.Sprintf("%s-%s-%s.pdf", part.Filename, msg.Id, part.PartId)
}

func processPDFFile(srv *gmail.Service, userID string, part *gmail.MessagePart, msg *gmail.Message) error {
	// Retrieve the attachment
	body, err := retrieveAttachment(srv, userID, msg, part.Body)
	if err != nil {
		return err
	}

	filename := constructFilename(part, msg)
	f, err := os.OpenFile(filename, os.O_TRUNC|os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	// Decode base64 encoded data
	fileContent, err := base64.URLEncoding.DecodeString(body.Data)
	if err != nil {
		return err
	}

	if _, err := f.Write(fileContent); err != nil {
		return err
	}
	log.Printf("Successfully written file: %s\n", filename)
	return nil
}

func retrieveAttachment(srv *gmail.Service, userID string, msg *gmail.Message, body *gmail.MessagePartBody) (*gmail.MessagePartBody, error) {
	if body.AttachmentId != "" {
		// make a http request for the body
		log.Printf("Requesting for attachment: %s\n", body.AttachmentId)
		call := srv.Users.Messages.Attachments.Get(userID, msg.Id, body.AttachmentId)
		return call.Do()
	}
	return body, nil
}

func markAsRead(srv *gmail.Service, userID string, msgs []*gmail.Message) error {
	msgIds := make([]string, len(msgs))
	for i, msg := range msgs {
		msgIds[i] = msg.Id
	}

	req := &gmail.BatchModifyMessagesRequest{
		Ids:            msgIds,
		RemoveLabelIds: []string{"UNREAD"},
	}

	call := srv.Users.Messages.BatchModify(userID, req)
	return call.Do()
}

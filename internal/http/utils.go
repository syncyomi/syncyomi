package http

import (
	"bytes"
	"compress/gzip"
	"io"
)

var Repo *Server

func uncompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer func(reader *gzip.Reader) {
		err := reader.Close()
		if err != nil {
			Repo.log.Error().Err(err).Msg("failed to close gzip reader")
		}
	}(reader)

	uncompressedData, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return uncompressedData, nil
}

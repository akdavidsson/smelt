package output

import (
	"fmt"
	"io"

	"github.com/akdavidsson/smelt/internal/extract"
)

// WriteParquet is a stub — parquet output is not yet implemented.
func WriteParquet(w io.Writer, result *extract.Result) error {
	return fmt.Errorf("parquet output is not yet implemented")
}

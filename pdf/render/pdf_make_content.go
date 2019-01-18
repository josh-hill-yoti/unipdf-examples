/*
 * Draw a line in a new PDF file.
 *
 * Run as: go run pdf_draw_line.go <x1> <y1> <x2> <y2> output.pdf
 * The x, y coordinates start from the upper left corner at (0,0) and increase going right, down respectively.
 */

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/creator"
	"github.com/unidoc/unidoc/pdf/model"
)

func main() {
	// If debugging:
	common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))

	if len(os.Args) < 2 {
		fmt.Printf("go run pdf_make_content.go output.pdf\n")
		os.Exit(1)
	}

	outputPath := os.Args[1]

	err := writeContentToPDF(outputPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Complete, see output file: %s\n", outputPath)
}

func writeContentToPDF(outputPath string) error {
	// New creator with default properties (pagesize letter default).
	c := creator.New()

	page := model.NewPdfPage()
	mbox := model.PdfRectangle{
		Llx: 0,
		Lly: 0,
		Urx: 612,
		Ury: 812,
	}
	page.MediaBox = &mbox

	csdata, err := ioutil.ReadFile(`/tmp/cs2`)
	if err != nil {
		return err
	}

	err = page.AddContentStreamByString(string(csdata))
	if err != nil {
		return err
	}

	/*
			// A1 graphics state.
			gsA1 := core.MakeDict()
			gsA1.Set("CA", core.MakeFloat(0))
			gsA1.Set("ca", core.MakeFloat(1))
			page.Resources.AddExtGState(`A1`, gsA1)
			// A2 graphics state.
			gsA2 := core.MakeDict()
			gsA2.Set("CA", core.MakeFloat(1))
			gsA2.Set("ca", core.MakeFloat(1))
			page.Resources.AddExtGState(`A2`, gsA1)

			_ = page.AddContentStreamByString(`
		q
		1 j
		1 g
		0 j
		0 w
		1 G
		0 0 m
		576 0 l
		576 432 l
		0 432 l
		h
		f
		/A1 gs
		0 G
		122.4 43.2 m
		468 43.2 l
		468 388.8 l
		122.4 388.8 l
		h
		f
		Q
		q
		/A2 gs
		2 J
		1 w
		0 G
		122.4 388.8 m
		468 388.8 l
		468 388.8 l
		S
		468 43.2 m
		468 388.8 l
		468 388.8 l
		S
		122.4 43.2 m
		468 43.2 l
		468 43.2 l
		S
		122.4 43.2 m
		122.4 388.8 l
		122.4 388.8 l
		S
		/A2 gs
		0 J
		0 g
		1 j
		0.5 w
		122.4 43.2 m
		122.4 47.2 l
		B
		Q
		`)
	*/
	/*
			// B render.
			_ = page.AddContentStreamByString(`
		q
		0.1 0 0 0.1 0 0 cm
		1 0 0 rg
		4.17188 10 m
		348.867 10 l
		515.543 10 559.762 38.3477 606.25 83.6992 c
		649.34 125.652 676.551 186.883 676.551 251.512 c
		676.551 332.016 650.473 411.387 550.691 448.805 c
		584.707 465.812 650.473 498.691 650.473 615.48 c
		650.473 699.387 599.449 824.109 399.891 824.109 c
		4.17188 824.109 l
		h
		%f
		0 1 0 rg
		167.449 368.301 m
		384.016 368.301 l
		445.242 368.301 506.473 341.086 506.473 269.652 c
		506.473 186.883 458.852 150.598 376.078 150.598 c
		167.449 150.598 l
		h
		%f
		0 0 1 rg
		167.449 683.512 m
		362.473 683.512 l
		439.574 683.512 487.195 663.102 487.195 596.203 c
		487.195 531.574 433.906 505.496 367.008 505.496 c
		167.449 505.496 l
		h
		f
		Q
		`)
	*/
	err = c.AddPage(page)
	if err != nil {
		return err
	}

	return c.WriteToFile(outputPath)
}

/*
 * Render PDF pages to images.
 *
 * Run as: go run pdf_render.go input.pdf
 */

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang/freetype/truetype"

	"golang.org/x/image/font"

	"github.com/gunnsth/gg"

	"github.com/unidoc/unidoc/common"
	pdfcontent "github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model"
	pdf "github.com/unidoc/unidoc/pdf/model"
)

var xObjectImages = 0
var inlineImages = 0

func main() {
	// Enable debug-level console logging, when debuggingn:
	common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))

	if len(os.Args) < 3 {
		fmt.Printf("Syntax: go run pdf_render.go file.pdf page\n")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	pageNum, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Render page %d of %s\n", pageNum, inputPath)
	err = renderPdfPage(inputPath, pageNum)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func renderPdfPage(inputPath string, pageNum int) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	pdfReader, err := pdf.NewPdfReader(f)
	if err != nil {
		return err
	}

	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return err
	}

	// Try decrypting with an empty one.
	if isEncrypted {
		auth, err := pdfReader.Decrypt([]byte(""))
		if err != nil {
			// Encrypted and we cannot do anything about it.
			return err
		}
		if !auth {
			fmt.Println("Need to decrypt with password")
			return nil
		}
	}

	page, err := pdfReader.GetPage(pageNum)
	if err != nil {
		return err
	}

	mbox, err := page.GetMediaBox()
	if err != nil {
		return err
	}

	// TODO(gunnsth): Allow arbitrary dimension/resolution of output image.
	ctx := &renderContext{
		dc:     gg.NewContext(int(mbox.Width()), int(mbox.Height())),
		Width:  mbox.Width(),
		Height: mbox.Height(),
	}
	// Fill with white.
	ctx.dc.Push()
	ctx.dc.SetRGBA(1, 1, 1, 1)
	ctx.dc.DrawRectangle(0, 0, float64(ctx.dc.Width()), float64(ctx.dc.Height()))
	ctx.dc.Fill()
	ctx.dc.Pop()

	// Lower left corner.
	ctx.dc.Translate(0, ctx.Height)

	// Set default
	ctx.dc.SetLineWidth(1.0)
	ctx.dc.SetRGBA(0, 0, 0, 1)

	err = renderPage(page, ctx)
	if err != nil {
		common.Log.Debug("Render error: %v", err)
		return err
	}

	// Save output as png image.
	err = ctx.dc.SavePNG(`/tmp/out.png`)
	if err != nil {
		return err
	}

	return nil
}

type renderContext struct {
	Width  float64
	Height float64
	dc     *gg.Context
	fface  font.Face
}

func renderPage(page *pdf.PdfPage, ctx *renderContext) error {
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return err
	}

	return renderContentStream(contents, page.Resources, ctx)
}

var (
	errType  = errors.New("type check error")
	errRange = errors.New("range check error")
)

func renderContentStream(contents string, resources *pdf.PdfPageResources, ctx *renderContext) error {
	cstreamParser := pdfcontent.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return err
	}

	processor := pdfcontent.NewContentStreamProcessor(*operations)
	processor.AddHandler(pdfcontent.HandlerConditionEnumAllOperands, "",
		func(op *pdfcontent.ContentStreamOperation, gs pdfcontent.GraphicsState, resources *pdf.PdfPageResources) error {
			common.Log.Debug("Processing %s", op.Operand)
			switch op.Operand {
			//
			// Graphics stage operators.
			//
			case "q":
				ctx.dc.Push()
			case "Q":
				ctx.dc.Pop()
			case "cm": // concatenate matrix
				if len(op.Params) != 6 {
					return errRange
				}
				fv, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}

				//XX, YX, XY, YY, X0, Y0 float64
				m := gg.Matrix{
					XX: fv[0],
					YX: fv[1],
					XY: fv[2],
					YY: fv[3],
					X0: fv[4],
					//Y0: ctx.Height - fv[5],
					Y0: -fv[5],
				}
				common.Log.Debug("m: %+v\n", m)
				ctx.dc.ConcatMatrix(m)

				// TODO(gunnsth): Incorrect handling of linewidth. Should take angle into account (8.4.3.2 Line Width).
				s := (gs.CTM.ScalingFactorX() + gs.CTM.ScalingFactorY()) / 2.0
				w := ctx.dc.GetLineWidth() * s
				ctx.dc.SetLineWidth(w)
				common.Log.Debug("Setting line width to: %v", w)
			case "w": // line width
				if len(op.Params) != 1 {
					return errRange
				}
				fw, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				// TODO(gunnsth): Incorrect handling of linewidth. Should take angle into account (8.4.3.2 Line Width).
				s := (gs.CTM.ScalingFactorX() + gs.CTM.ScalingFactorY()) / 2.0
				common.Log.Debug("Set line width: %v", s*fw[0])
				ctx.dc.SetLineWidth(s * fw[0])
			case "J": // line cap style
				if len(op.Params) != 1 {
					return errRange
				}
				val, ok := core.GetIntVal(op.Params[0])
				if !ok {
					return errType
				}
				switch val {
				case 0: // butt cap
					ctx.dc.SetLineCap(gg.LineCapButt)
				case 1: // round cap
					ctx.dc.SetLineCap(gg.LineCapRound)
				case 2: // projecting square cap
					// TODO(gunnsth): Check if same as "projecting square"
					ctx.dc.SetLineCap(gg.LineCapSquare)
				default:
					common.Log.Debug("Invalid line cap style")
					return errRange
				}
			case "j": // line join style
				if len(op.Params) != 1 {
					return errRange
				}
				val, ok := core.GetIntVal(op.Params[0])
				if !ok {
					return errType
				}
				switch val {
				case 0: // miter join
					// TODO(gunnsth): gg does not support 'miter' join.
					ctx.dc.SetLineJoin(gg.LineJoinBevel)
				case 1: // round join.
					ctx.dc.SetLineJoin(gg.LineJoinRound)
				case 2: // bevel join.
					ctx.dc.SetLineJoin(gg.LineJoinBevel)
				default:
					common.Log.Debug("Invalid line join style")
					return errRange
				}
			case "M": // set miter limit
				if len(op.Params) != 1 {
					return errRange
				}
				fw, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				// TODO(gunnsth): Add miter support in gg.
				_ = fw
				// ctx.dc.SetMiterLimit(fw[0])
			case "d": // set line dash pattern
				if len(op.Params) != 2 {
					return errRange
				}
				dashArray, ok := core.GetArray(op.Params[0])
				if !ok {
					return errType
				}
				phase, ok := core.GetIntVal(op.Params[1])
				if !ok {
					return errType
				}
				dashes, err := core.GetNumbersAsFloat(dashArray.Elements())
				if err != nil {
					return err
				}
				ctx.dc.SetDash(dashes...)
				//ctx.dc.SetDashPhase(phase)
				// TODO(gunnsth): Add support for dash phase in gg.
				_ = phase
			case "ri": // set color rendering intent
				common.Log.Debug("Rendering intent not supported")
				// TODO(gunnsth): Add support for rendering intent.
			case "i": // set flatness tolerance.
				// TODO(gunnsth): Implement flatness tolerance in gg.
				common.Log.Debug("Flatness tolerance not supported")
			case "gs": // set graphics state from dict.
				// TODO(gunnsth): Should not need this here, the `gs` should be updated before handler called.
				if len(op.Params) != 1 {
					return errRange
				}
				rname, ok := core.GetName(op.Params[0])
				if !ok {
					return errType
				}
				if rname == nil {
					return errRange
				}
				// TODO(gunnsth): Support graphics state loaading from resources.
				//ctx.GraphicsState(rname)
				common.Log.Debug("TODO: gs: add support, tried to load %s (ExtGState)", rname.String())
				extobj, ok := resources.GetExtGState(*rname)
				if !ok {
					return errors.New("resource not found")
				}
				extdict, ok := core.GetDict(extobj)
				if !ok {
					return errType
				}
				common.Log.Debug("GS dict: %s", extdict.String())

			//
			// Path operators.
			//
			case "m": // move to.
				if len(op.Params) != 2 {
					return errRange
				}
				xy, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				//ctx.dc.MoveTo(xy[0], ctx.Height-xy[1])
				//ctx.dc.ClearPath()
				ctx.dc.NewSubPath()
				ctx.dc.MoveTo(xy[0], -xy[1])
			case "l": // line to.
				if len(op.Params) != 2 {
					return errRange
				}
				xy, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				//ctx.dc.LineTo(xy[0], ctx.Height-xy[1])
				ctx.dc.LineTo(xy[0], -xy[1])
			case "c": // cubic bezier
				if len(op.Params) != 6 {
					return errRange
				}
				cbp, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				// TODO: Check if same as defined in PDF.
				common.Log.Debug("Cubic bezier params: %+v", cbp)
				//ctx.dc.CubicTo(cbp[0], ctx.Height-cbp[1], cbp[2], ctx.Height-cbp[3], cbp[4], ctx.Height-cbp[5])
				ctx.dc.CubicTo(cbp[0], -cbp[1], cbp[2], -cbp[3], cbp[4], -cbp[5])
			case "v": // cubic bezier
				if len(op.Params) != 4 {
					return errRange
				}
				cbp, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				//curPos, _ := ctx.dc.Current()
				//ctx.dc.CubicTo(curPos.X, curPos.Y, cbp[0], -cbp[1], cbp[2], -cbp[3])
				ctx.dc.CubicTo(0, 0, cbp[0], -cbp[1], cbp[2], -cbp[3])
			case "h":
				ctx.dc.ClosePath()
				ctx.dc.NewSubPath()
			case "re": // rectangle
				if len(op.Params) != 4 {
					return errRange
				}
				xywh, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				//ctx.dc.Translate(0, -xywh[3])
				ctx.dc.DrawRectangle(xywh[0], -xywh[1], xywh[2], -xywh[3])
				ctx.dc.NewSubPath()

			//
			// Path painting operators.
			//
			case "S": // stroke the path
				color, err := gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)

				ctx.dc.Stroke()

			case "s": // close and stroke
				color, err := gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)

				ctx.dc.Stroke()
			case "f", "F": // fill with nonzero winding number rule
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				ctx.dc.SetFillRule(gg.FillRuleWinding)
				ctx.dc.Fill()
			case "f*": // fill with even odd rule
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				ctx.dc.SetFillRule(gg.FillRuleEvenOdd)
				ctx.dc.Fill()
			case "B": // fill then stroke the path (nonzero winding rule)
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				common.Log.Debug("Filling")
				ctx.dc.SetFillRule(gg.FillRuleWinding)
				ctx.dc.FillPreserve()

				color, err = gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor = color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)
				ctx.dc.Stroke()
			case "B*": // fill then stroke the path (even odd rule)
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				ctx.dc.SetFillRule(gg.FillRuleEvenOdd)
				ctx.dc.FillPreserve() // XXX

				color, err = gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor = color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)
				ctx.dc.Stroke() // XXX
			case "b": // Close, fill and stroke the path (nonzero winding rule).
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				ctx.dc.ClosePath()
				ctx.dc.NewSubPath() // TODO: needed?
				ctx.dc.SetFillRule(gg.FillRuleWinding)
				ctx.dc.FillPreserve()

				color, err = gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor = color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)
				ctx.dc.Stroke()
			case "b*": // Close, fill and stroke the path (even odd rule).
				color, err := gs.ColorspaceNonStroking.ColorToRGB(gs.ColorNonStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor := color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaNonStroking)

				ctx.dc.ClosePath()
				ctx.dc.NewSubPath() // TODO: needed?
				ctx.dc.SetFillRule(gg.FillRuleEvenOdd)
				ctx.dc.FillPreserve()

				color, err = gs.ColorspaceStroking.ColorToRGB(gs.ColorStroking)
				if err != nil {
					common.Log.Debug("Error converting color: %v", err)
					return err
				}
				rgbColor = color.(*pdf.PdfColorDeviceRGB)
				ctx.dc.SetRGBA(rgbColor.R(), rgbColor.G(), rgbColor.B(), gs.AlphaStroking)
				ctx.dc.Stroke()
			case "n": // End the current path without filling or stroking.
				ctx.dc.ClearPath()
				//ctx.dc.ResetClip()

			//
			// Clipping path operators.
			//
			case "W": // modify current clipping path (nonzero winding).
				ctx.dc.SetFillRule(gg.FillRuleWinding)
				ctx.dc.ClipPreserve()
				// TODO(gunnsth): Fix clipping.  Currently not working properly.
				//ctx.dc.StrokePreserve()
				//ctx.dc.Clip()

			case "W*": // modify current clipping path (even odd winding).
				ctx.dc.SetFillRule(gg.FillRuleEvenOdd)
				//common.Log.Debug("W* Clipping: ", ctx.dc.ResetClip)
				ctx.dc.ClipPreserve()
				//ctx.dc.StrokePreserve()
				//ctx.dc.Clip()
				// TODO(gunnsth): Fix clipping.  Currently not working properly.

			//
			// Color oprerators.
			//
			//case "G", "g":

			//
			// Image operators.
			//
			case "Do":
				if len(op.Params) != 1 {
					return errRange
				}

				// Do: XObject.
				name, ok := core.GetName(op.Params[0])
				if !ok {
					return errType
				}

				_, xtype := resources.GetXObjectByName(*name)
				if xtype == pdf.XObjectTypeImage {
					common.Log.Debug(" XObject Image: %s", name.String())

					ximg, err := resources.GetXObjectImageByName(*name)
					if err != nil {
						return err
					}

					img, err := ximg.ToImage()
					if err != nil {
						return err
					}

					goImg, err := img.ToGoImage()
					if err != nil {
						return err
					}

					bounds := goImg.Bounds()

					ctx.dc.Push()
					ctx.dc.Scale(1.0/float64(bounds.Dx()), 1.0/float64(bounds.Dy()))

					// TODO(gunnsth): Handle soft masks properly. Needs some refactoring within unidoc as well.

					/*
						if ximg.SMask != nil {
							stream, ok := core.GetStream(ximg.SMask)
							if !ok {
								return errType
							}
							smask, err := pdf.NewXObjectImageFromStream(stream)
							if err != nil {
								return err
							}

							smaskImg, err := smask.ToImage()
							if err != nil {
								return err
							}
							smaskImg.GetSamples()
							clip := image.NewAlpha(image.Rect(0, 0, dc.width, dc.height))
							clip.
						}
					*/

					//curPos, _ := ctx.dc.Current()
					//common.Log.Debug("Draw image at (%f,%f)", curPos.X, curPos.Y)
					ctx.dc.DrawImageAnchored(goImg, 0, 0, 0, 1)

					ctx.dc.Pop()
				} else if xtype == pdf.XObjectTypeForm {
					common.Log.Debug(" XObject Form: %s", name.String())
					// Go through the XObject Form content stream.
					xform, err := resources.GetXObjectFormByName(*name)
					if err != nil {
						return err
					}

					formContent, err := xform.GetContentStream()
					if err != nil {
						return err
					}

					// Process the content stream in the Form object too:
					formResources := xform.Resources
					if formResources == nil {
						formResources = resources
					}

					ctx.dc.Push()
					if xform.Matrix != nil {
						array, ok := core.GetArray(xform.Matrix)
						if !ok {
							return errType
						}
						mf, err := core.GetNumbersAsFloat(array.Elements())
						if err != nil {
							return err
						}
						if len(mf) != 6 {
							return errRange
						}
						m := gg.Matrix{
							XX: mf[0],
							YX: mf[1],
							XY: mf[2],
							YY: mf[3],
							X0: mf[4],
							Y0: -mf[5],
						}
						ctx.dc.ConcatMatrix(m)
					}
					if xform.BBox != nil {
						array, ok := core.GetArray(xform.BBox)
						if !ok {
							return errType
						}
						bf, err := core.GetNumbersAsFloat(array.Elements())
						if err != nil {
							return err
						}
						if len(bf) != 4 {
							common.Log.Debug("Len = %d", len(bf))
							return errRange
						}
						// Set clipping region.
						ctx.dc.DrawRectangle(bf[0], -bf[1], bf[2]-bf[0], -(bf[3] - bf[1]))
						ctx.dc.SetRGB(1, 0, 0)
						// For debugging bbox, uncomment:
						//ctx.dc.StrokePreserve()
						ctx.dc.Clip()
					} else {
						common.Log.Debug("ERROR: Required BBox missing on XObject Form")
					}

					// Process the content stream in the Form object too:
					err = renderContentStream(string(formContent), formResources, ctx)
					if err != nil {
						return err
					}
					ctx.dc.Pop()
				}

			case "BI": // inline image
				if len(op.Params) != 1 {
					return errRange
				}
				iimg, ok := op.Params[0].(*pdfcontent.ContentStreamInlineImage)
				if !ok {
					return nil
				}

				img, err := iimg.ToImage(resources)
				if err != nil {
					return err
				}

				goImg, err := img.ToGoImage()
				if err != nil {
					return err
				}

				bounds := goImg.Bounds()

				ctx.dc.Push()
				ctx.dc.Scale(1.0/float64(bounds.Dx()), 1.0/float64(bounds.Dy()))

				//curPos, _ := ctx.dc.Current()
				//common.Log.Debug("Draw image at (%f,%f)", curPos.X, curPos.Y)
				ctx.dc.DrawImageAnchored(goImg, 0, 0, 0, 1)

				ctx.dc.Pop()

			case "BT":
				ctx.dc.Push()
			case "ET":
				ctx.dc.Pop()
			case "Td":
				if len(op.Params) != 2 {
					return errRange
				}
				fv, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				// TODO(gunnsth): Account for CTM / text matrix ?
				ctx.dc.MoveTo(fv[0], -fv[1])
			case "TD":
				if len(op.Params) != 2 {
					return errRange
				}
				fv, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				// TODO(gunnsth): Account for CTM / text matrix ?
				ctx.dc.MoveTo(fv[0], 10-fv[1])
			case "Tm":
				if len(op.Params) != 6 {
					return errRange
				}
				fv, err := core.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				fmt.Printf("Tm = % d\n", fv)
			case `'`:
				// TODO: Use lineheight
				ctx.dc.MoveTo(0, 10)
				if len(op.Params) != 1 {
					return errRange
				}
				charcodes, ok := core.GetStringBytes(op.Params[0])
				if !ok {
					return errType
				}
				// TODO(gunnsth): Account for encoding.
				ctx.dc.DrawString(string(charcodes), 0, 0)
			case `''`:
				ctx.dc.MoveTo(0, 10)
				if len(op.Params) != 3 {
					return errRange
				}
				charcodes, ok := core.GetStringBytes(op.Params[2])
				if !ok {
					return errType
				}
				// TODO(gunnsth): Account for encoding.
				ctx.dc.DrawString(string(charcodes), 0, 0)

			case "Tj":
				fmt.Printf("Tj\n")
				if len(op.Params) != 1 {
					return errRange
				}
				charcodes, ok := core.GetStringBytes(op.Params[0])
				if !ok {
					return errType
				}
				// TODO(gunnsth): Account for encoding.
				ctx.dc.DrawString(string(charcodes), 0, 0)
			case "TJ":
				fmt.Printf("TJ\n")
				if len(op.Params) != 1 {
					return errRange
				}
				array, ok := core.GetArray(op.Params[0])
				if !ok {
					common.Log.Debug("Type: %T", array)
					return errType
				}
				for _, obj := range array.Elements() {
					switch t := obj.(type) {
					case *core.PdfObjectString:
						if t != nil {
							ctx.dc.DrawString(t.String(), 0, 0)
						}
					case *core.PdfObjectFloat:
						if t != nil {
							ctx.dc.MoveTo(float64(*t), 0)
						}
					case *core.PdfObjectInteger:
						if t != nil {
							ctx.dc.MoveTo(float64(*t), 0)
						}
					}
				}

			case "Tf":
				fmt.Printf("Tf\n")
				if len(op.Params) != 2 {
					return errRange
				}
				common.Log.Debug("%#v", op.Params)
				fname, ok := core.GetName(op.Params[0])
				if !ok || fname == nil {
					return errType
				}
				common.Log.Debug("Font name: %s", fname.String())
				fsize, err := core.GetNumberAsFloat(op.Params[1])
				if err != nil {
					return errType
				}
				common.Log.Debug("Font size: %v", fsize)

				fObj, has := resources.GetFontByName(*fname)
				if !has {
					common.Log.Debug("ERROR: Font %s not found", fname.String())
					return errors.New("font not found")
				}
				common.Log.Debug("font: %T", fObj)

				fontDict, ok := core.GetDict(fObj)
				if !ok {
					return errType
				}
				fmt.Printf("%s\n", fontDict.String())

				pdfFont, err := model.NewPdfFontFromPdfObject(fObj)
				if err != nil {
					common.Log.Debug("Error loading font")
					return err
				}
				fmt.Printf("%#v\n", pdfFont)
				fmt.Printf("%s\n", pdfFont.BaseFont())
				fmt.Printf("%s\n", pdfFont.String())
				fmt.Printf("%s\n", pdfFont.Subtype())

				// TODO(gunnsth): Why does font have both FontDescriptor() and GetFontDescriptor() methods?
				descriptor, err := pdfFont.GetFontDescriptor()
				if err != nil {
					return err
				}
				if descriptor == nil {
					return errors.New("font descriptor missing")
				}
				fmt.Printf("Descriptor: %#v\n", descriptor)

				fontStream, ok := core.GetStream(descriptor.FontFile2)
				if !ok {
					// TODO: Handle simple fonts by loading fonts from system.
					return nil // Ignore for now
					//return errType
				}

				fontData, err := core.DecodeStream(fontStream)
				if err != nil {
					return err
				}

				// TODO(gunnsth): Cache font (use object number or pointer?).
				tfont, err := truetype.Parse(fontData)
				if err != nil {
					common.Log.Debug("Error parsing font: %v", err)
					return err
				}
				face := truetype.NewFace(tfont, &truetype.Options{
					Size: fsize,
				})
				if err != nil {
					return err
				}
				ctx.fface = face
				ctx.dc.SetFontFace(face)
				fmt.Printf("Loaded font face")

			default:
				common.Log.Debug("NOT SUPPORTED")
			}

			return nil
		})

	err = processor.Process(resources)
	if err != nil {
		return err
	}

	return nil
}

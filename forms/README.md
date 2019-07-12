## Issue: Calling Inspect() breaks flattening


### Scenario

When `pdfReader.Inspect()` is invoked before flattening, the output file is blank:

```gherkin
Scenario: Flattening all annotations in an encrypted PDF, with a call to pdfReader.Inspect()

Given an encrypted PDF 'read-only.pdf'
And this fork of 'pdf_form_flatten.go'
When I run 'UNIPDF_INSPECT=1 go run pdf_form_flatten.go . read-only.pdf'
Then I do not see an error in the output
And all the pages in 'flattened_read-only.pdf' are blank (except for the UniDoc watermark)
```

### Steps to reproduce

Clone this fork on `unipdf-examples`:
```bash
$ git clone git@github.com:josh-hill-yoti/unipdf-examples.git
$ cd unipdf-examples/forms
```

Flatten `read-only.pdf`:
```bash
$ go run pdf_form_flatten.go . read-only.pdf
[DEBUG]  parser.go:747 Pdf version 1.5
Unlicensed copy of unidoc
To get rid of the watermark - Please get a license on https://unidoc.io
Total 1 processed / 0 failures
```

Inspect flattened PDF and see all pages are blank:
```bash
$ open flattened_read-only.pdf
```

### Expected behaviour

I expected the output PDF `flattened_read-only.pdf` to have the same visible content as `read-only.pdf`.

Or if flattening is not possible I would expect an error to be returned.

### Workaround

Create a new `pdfReader` for flattening:
```bash
$ WORKAROUND=1 go run pdf_form_flatten.go . read-only.pdf
[DEBUG]  parser.go:747 Pdf version 1.5
[DEBUG]  parser.go:747 Pdf version 1.5
Unlicensed copy of unidoc
To get rid of the watermark - Please get a license on https://unidoc.io
Total 1 processed / 0 failures
```

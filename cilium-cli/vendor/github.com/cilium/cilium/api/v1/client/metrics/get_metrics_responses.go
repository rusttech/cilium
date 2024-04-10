// Code generated by go-swagger; DO NOT EDIT.

// Copyright Authors of Cilium
// SPDX-License-Identifier: Apache-2.0

package metrics

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/cilium/cilium/api/v1/models"
)

// GetMetricsReader is a Reader for the GetMetrics structure.
type GetMetricsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetMetricsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewGetMetricsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 500:
		result := NewGetMetricsInternalServerError()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		return nil, runtime.NewAPIError("[GET /metrics/] GetMetrics", response, response.Code())
	}
}

// NewGetMetricsOK creates a GetMetricsOK with default headers values
func NewGetMetricsOK() *GetMetricsOK {
	return &GetMetricsOK{}
}

/*
GetMetricsOK describes a response with status code 200, with default header values.

Success
*/
type GetMetricsOK struct {
	Payload []*models.Metric
}

// IsSuccess returns true when this get metrics o k response has a 2xx status code
func (o *GetMetricsOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this get metrics o k response has a 3xx status code
func (o *GetMetricsOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this get metrics o k response has a 4xx status code
func (o *GetMetricsOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this get metrics o k response has a 5xx status code
func (o *GetMetricsOK) IsServerError() bool {
	return false
}

// IsCode returns true when this get metrics o k response a status code equal to that given
func (o *GetMetricsOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the get metrics o k response
func (o *GetMetricsOK) Code() int {
	return 200
}

func (o *GetMetricsOK) Error() string {
	return fmt.Sprintf("[GET /metrics/][%d] getMetricsOK  %+v", 200, o.Payload)
}

func (o *GetMetricsOK) String() string {
	return fmt.Sprintf("[GET /metrics/][%d] getMetricsOK  %+v", 200, o.Payload)
}

func (o *GetMetricsOK) GetPayload() []*models.Metric {
	return o.Payload
}

func (o *GetMetricsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// response payload
	if err := consumer.Consume(response.Body(), &o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGetMetricsInternalServerError creates a GetMetricsInternalServerError with default headers values
func NewGetMetricsInternalServerError() *GetMetricsInternalServerError {
	return &GetMetricsInternalServerError{}
}

/*
GetMetricsInternalServerError describes a response with status code 500, with default header values.

Metrics cannot be retrieved
*/
type GetMetricsInternalServerError struct {
}

// IsSuccess returns true when this get metrics internal server error response has a 2xx status code
func (o *GetMetricsInternalServerError) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this get metrics internal server error response has a 3xx status code
func (o *GetMetricsInternalServerError) IsRedirect() bool {
	return false
}

// IsClientError returns true when this get metrics internal server error response has a 4xx status code
func (o *GetMetricsInternalServerError) IsClientError() bool {
	return false
}

// IsServerError returns true when this get metrics internal server error response has a 5xx status code
func (o *GetMetricsInternalServerError) IsServerError() bool {
	return true
}

// IsCode returns true when this get metrics internal server error response a status code equal to that given
func (o *GetMetricsInternalServerError) IsCode(code int) bool {
	return code == 500
}

// Code gets the status code for the get metrics internal server error response
func (o *GetMetricsInternalServerError) Code() int {
	return 500
}

func (o *GetMetricsInternalServerError) Error() string {
	return fmt.Sprintf("[GET /metrics/][%d] getMetricsInternalServerError ", 500)
}

func (o *GetMetricsInternalServerError) String() string {
	return fmt.Sprintf("[GET /metrics/][%d] getMetricsInternalServerError ", 500)
}

func (o *GetMetricsInternalServerError) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

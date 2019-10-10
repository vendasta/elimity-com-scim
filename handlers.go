package scim

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/elimity-com/scim/errors"
)

func errorHandler(w http.ResponseWriter, r *http.Request, scimErr scimError) {
	raw, err := json.Marshal(scimErr)
	if err != nil {
		log.Fatalf("failed marshaling scim error: %v", err)
	}
	w.WriteHeader(scimErr.status)
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// schemasHandler receives an HTTP GET to retrieve information about resource schemas supported by a SCIM service
// provider. An HTTP GET to the endpoint "/Schemas" returns all supported schemas in ListResponse format.
func (s Server) schemasHandler(w http.ResponseWriter, r *http.Request) {
	var schemas []interface{}
	for _, v := range s.getSchemas() {
		schemas = append(schemas, v)
	}

	raw, err := json.Marshal(newListResponse(schemas))
	if err != nil {
		log.Fatalf("failed marshaling list response: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// schemaHandler receives an HTTP GET to retrieve individual schema definitions which can be returned by appending the
// schema URI to the /Schemas endpoint. For example: "/Schemas/urn:ietf:params:scim:schemas:core:2.0:User"
func (s Server) schemaHandler(w http.ResponseWriter, r *http.Request, id string) {
	schema, ok := s.getSchemas()[id]
	if !ok {
		errorHandler(w, r, scimErrorResourceNotFound(id))
		return
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		log.Fatalf("failed marshaling schema: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourceTypesHandler receives an HTTP GET to this endpoint, "/ResourceTypes", which is used to discover the types of
// resources available on a SCIM service provider (e.g., Users and Groups).  Each resource type defines the endpoints,
// the core schema URI that defines the resource, and any supported schema extensions.
func (s Server) resourceTypesHandler(w http.ResponseWriter, r *http.Request) {
	var resourceTypes []interface{}
	for _, v := range s.ResourceTypes {
		resourceTypes = append(resourceTypes, v)
	}

	raw, err := json.Marshal(newListResponse(resourceTypes))
	if err != nil {
		log.Fatalf("failed marshaling list response: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourceTypeHandler receives an HTTP GET to retrieve individual resource types which can be returned by appending the
// resource types name to the /ResourceTypes endpoint. For example: "/ResourceTypes/User"
func (s Server) resourceTypeHandler(w http.ResponseWriter, r *http.Request, name string) {
	var resourceType ResourceType
	for _, r := range s.ResourceTypes {
		if r.Name == name {
			resourceType = r
			break
		}
	}
	if resourceType.Name != name {
		errorHandler(w, r, scimErrorResourceNotFound(name))
		return
	}

	raw, err := json.Marshal(resourceType)
	if err != nil {
		log.Fatalf("failed marshaling resource type: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// serviceProviderConfigHandler receives an HTTP GET to this endpoint will return a JSON structure that describes the
// SCIM specification features available on a service provider.
func (s Server) serviceProviderConfigHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := json.Marshal(s.Config)
	if err != nil {
		log.Fatalf("failed marshaling service provider config: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourcePostHandler receives an HTTP POST request to the resource endpoint, such as "/Users" or "/Groups", as
// defined by the associated resource type endpoint discovery to create new resources.
func (s Server) resourcePostHandler(w http.ResponseWriter, r *http.Request, resourceType ResourceType) {
	data, _ := ioutil.ReadAll(r.Body)

	attributes, scimErr := resourceType.validate(data)
	if scimErr != errors.ValidationErrorNil {
		errorHandler(w, r, scimValidationError(scimErr))
		return
	}

	resource, postErr := resourceType.Handler.Create(attributes)
	if postErr != errors.PostErrorNil {
		errorHandler(w, r, scimPostError(postErr))
		return
	}

	raw, err := json.Marshal(resource.response(resourceType))
	if err != nil {
		log.Fatalf("failed marshaling resource: %v", err)
	}
	w.WriteHeader(http.StatusCreated)
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourceGetHandler receives an HTTP GET request to the resource endpoint, e.g., "/Users/{id}" or "/Groups/{id}",
// where "{id}" is a resource identifier to retrieve a known resource.
func (s Server) resourceGetHandler(w http.ResponseWriter, r *http.Request, id string, resourceType ResourceType) {
	resource, getErr := resourceType.Handler.Get(id)
	if getErr != errors.GetErrorNil {
		errorHandler(w, r, scimGetError(getErr, id))
		return
	}

	raw, err := json.Marshal(resource.response(resourceType))
	if err != nil {
		errorHandler(w, r, scimErrorInternalServer)
		log.Fatalf("failed marshaling resource: %v", err)
		return
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourcesGetHandler receives an HTTP GET request to the resource endpoint, e.g., "/Users" or "/Groups", to retrieve
// all known resources.
func (s Server) resourcesGetHandler(w http.ResponseWriter, r *http.Request, resourceType ResourceType, params ListRequestParams) {
	page, getError := resourceType.Handler.GetAll(params)

	if getError != errors.GetErrorNil {
		errorHandler(w, r, scimGetAllError(getError))
		return
	}

	raw, err := json.Marshal(
		page.toInternalListResponse(resourceType, params.StartIndex, params.Count),
	)

	if err != nil {
		errorHandler(w, r, scimErrorInternalServer)
		log.Fatalf("failed marshalling list response: %v", err)
		return
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourcePutHandler receives an HTTP PUT to the resource endpoint, e.g., "/Users/{id}" or "/Groups/{id}", where
// "{id}" is a resource identifier to replace a resource's attributes.
func (s Server) resourcePutHandler(w http.ResponseWriter, r *http.Request, id string, resourceType ResourceType) {
	data, _ := ioutil.ReadAll(r.Body)

	attributes, scimErr := resourceType.validate(data)
	if scimErr != errors.ValidationErrorNil {
		errorHandler(w, r, scimValidationError(scimErr))
		return
	}

	resource, putError := resourceType.Handler.Replace(id, attributes)
	if putError != errors.PutErrorNil {
		errorHandler(w, r, scimPutError(putError, id))
		return
	}

	raw, err := json.Marshal(resource.response(resourceType))
	if err != nil {
		log.Fatalf("failed marshaling resource: %v", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		log.Printf("failed writing response: %v", err)
	}
}

// resourceDeleteHandler receives an HTTP DELETE request to the resource endpoint, e.g., "/Users/{id}" or "/Groups/{id}",
// where "{id}" is a resource identifier to delete a known resource.
func (s Server) resourceDeleteHandler(w http.ResponseWriter, r *http.Request, id string, resourceType ResourceType) {
	deleteErr := resourceType.Handler.Delete(id)
	if deleteErr != errors.DeleteErrorNil {
		errorHandler(w, r, scimDeleteError(deleteErr, id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

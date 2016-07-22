// Copyright 2016 Mender Software AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package controller

import (
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/asaskevich/govalidator"
	"github.com/mendersoftware/deployments/resources/deployments"
	"github.com/mendersoftware/deployments/utils/identity"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

// Errors
var (
	ErrIDNotUUIDv4  = errors.New("ID is not UUIDv4")
	ErrDeploymentID = errors.New("Invalid deployment ID")
	ErrInternal     = errors.New("Internal error")
)

type DeploymentsController struct {
	view  RESTView
	model DeploymentsModel
}

func NewDeploymentsController(model DeploymentsModel, view RESTView) *DeploymentsController {
	return &DeploymentsController{
		view:  view,
		model: model,
	}
}

func (d *DeploymentsController) PostDeployment(w rest.ResponseWriter, r *rest.Request) {

	constructor, err := d.getDeploymentConstructorFromBody(r)
	if err != nil {
		d.view.RenderError(w, errors.Wrap(err, "Validating request body"), http.StatusBadRequest)
		return
	}

	id, err := d.model.CreateDeployment(constructor)
	if err != nil {
		d.view.RenderError(w, err, http.StatusInternalServerError)
		return
	}

	d.view.RenderSuccessPost(w, r, id)
}

func (d *DeploymentsController) getDeploymentConstructorFromBody(r *rest.Request) (*deployments.DeploymentConstructor, error) {

	var constructor *deployments.DeploymentConstructor
	if err := r.DecodeJsonPayload(&constructor); err != nil {
		return nil, err
	}

	if err := constructor.Validate(); err != nil {
		return nil, err
	}

	return constructor, nil
}

func (d *DeploymentsController) GetDeployment(w rest.ResponseWriter, r *rest.Request) {

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, ErrIDNotUUIDv4, http.StatusBadRequest)
		return
	}

	deployment, err := d.model.GetDeployment(id)
	if err != nil {
		d.view.RenderError(w, err, http.StatusInternalServerError)
		return
	}

	if deployment == nil {
		d.view.RenderErrorNotFound(w)
		return
	}

	d.view.RenderSuccessGet(w, deployment)
}

func (d *DeploymentsController) GetDeploymentStats(w rest.ResponseWriter, r *rest.Request) {

	id := r.PathParam("id")

	if !govalidator.IsUUIDv4(id) {
		d.view.RenderError(w, ErrIDNotUUIDv4, http.StatusBadRequest)
		return
	}

	stats, err := d.model.GetDeploymentStats(id)
	if err != nil {
		d.view.RenderError(w, err, http.StatusInternalServerError)
		return
	}

	if stats == nil {
		d.view.RenderErrorNotFound(w)
		return
	}

	d.view.RenderSuccessGet(w, stats)
}

func (d *DeploymentsController) GetDeploymentForDevice(w rest.ResponseWriter, r *rest.Request) {

	idata, err := identity.ExtractIdentityFromHeaders(r.Header)
	if err != nil {
		d.view.RenderError(w, err, http.StatusBadRequest)
		return
	}

	deployment, err := d.model.GetDeploymentForDevice(idata.Subject)
	if err != nil {
		d.view.RenderError(w, err, http.StatusInternalServerError)
		return
	}

	if deployment == nil {
		d.view.RenderNoUpdateForDevice(w)
		return
	}

	d.view.RenderSuccessGet(w, deployment)
}

func (d *DeploymentsController) PutDeploymentStatusForDevice(w rest.ResponseWriter, r *rest.Request) {

	did := r.PathParam("id")

	idata, err := identity.ExtractIdentityFromHeaders(r.Header)
	if err != nil {
		d.view.RenderError(w, err, http.StatusBadRequest)
		return
	}

	// receive request body
	var report statusReport

	err = r.DecodeJsonPayload(&report)
	if err != nil {
		d.view.RenderError(w, err, http.StatusBadRequest)
		return
	}

	status := report.Status
	if err := d.model.UpdateDeviceDeploymentStatus(did, idata.Subject, status); err != nil {
		d.view.RenderError(w, err, http.StatusInternalServerError)
		return
	}

	d.view.RenderEmptySuccessResponse(w)
}

func (d *DeploymentsController) GetDeviceStatusesForDeployment(w rest.ResponseWriter, r *rest.Request) {
	did := r.PathParam("id")

	if !govalidator.IsUUIDv4(did) {
		d.view.RenderError(w, ErrIDNotUUIDv4, http.StatusBadRequest)
		return
	}

	statuses, err := d.model.GetDeviceStatusesForDeployment(did)
	if err != nil {
		switch err {
		case ErrModelDeploymentNotFound:
			d.view.RenderError(w, err, http.StatusNotFound)
			return
		default:
			d.view.RenderError(w, ErrInternal, http.StatusInternalServerError)
			return
		}
	}

	d.view.RenderSuccessGet(w, statuses)
}

// Deployment as returned in deployment lookup query results
type LookupDeploymentResult struct {
	// deployment ID
	Id string `json:"id"`

	// Deployment creation time
	Created *time.Time `json:"created"`

	// Finished deplyment time
	Finished *time.Time `json:"finished,omitempty"`
	// Deployment name

	Name string `json:"name"`

	// Artifact name
	ArtifactName string `json:"artifact_name,omitempty"`

	// Status
	Status string `json:"status"`
}

func (d *DeploymentsController) LookupDeployment(w rest.ResponseWriter, r *rest.Request) {
	query := deployments.Query{}

	search := r.URL.Query().Get("search")
	if search != "" {
		query.SearchText = search
	}

	deps, err := d.model.LookupDeployment(query)
	if err != nil {
		d.view.RenderError(w, err, http.StatusBadRequest)
		return
	}

	res := make([]LookupDeploymentResult, len(deps))
	for i, dep := range deps {
		res[i].Id = *dep.Id
		res[i].Name = *dep.Name
		res[i].ArtifactName = *dep.ArtifactName
	}

	d.view.RenderSuccessGet(w, res)
}

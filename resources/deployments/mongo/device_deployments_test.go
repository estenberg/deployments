// Copyright 2018 Northern.tech AS
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

package mongo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/mendersoftware/go-lib-micro/identity"
	ctxstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/deployments/resources/deployments"
	. "github.com/mendersoftware/deployments/resources/deployments/mongo"
	"github.com/mendersoftware/deployments/utils/pointers"
)

func TestDeviceDeploymentStorageInsert(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestDeviceDeploymentStorageInsert in short mode.")
	}

	testCases := []struct {
		InputDeviceDeployment []*deployments.DeviceDeployment
		InputTenant           string
		OutputError           error
	}{
		{
			InputDeviceDeployment: nil,
			OutputError:           nil,
		},
		{
			InputDeviceDeployment: []*deployments.DeviceDeployment{nil, nil},
			OutputError:           ErrStorageInvalidDeviceDeployment,
		},
		{
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("bad bad", "bad bad bad"),
				deployments.NewDeviceDeployment("bad bad", "bad bad bad"),
			},
			OutputError: errors.New("Validating device deployment: DeploymentId: bad bad bad does not validate as uuidv4;"),
		},
		{
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("30b3e62c-9ec2-4312-a7fa-cff24cc7397a", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
				deployments.NewDeviceDeployment("bad bad", "bad bad bad"),
			},
			OutputError: errors.New("Validating device deployment: DeploymentId: bad bad bad does not validate as uuidv4;"),
		},
		{
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("30b3e62c-9ec2-4312-a7fa-cff24cc7397a", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
				deployments.NewDeviceDeployment("30b3e62c-9ec2-4312-a7fa-cff24cc7397a", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			OutputError: nil,
		},
		{
			// same as previous case, but this time with tenant DB
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("30b3e62c-9ec2-4312-a7fa-cff24cc7397a", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
				deployments.NewDeviceDeployment("30b3e62c-9ec2-4312-a7fa-cff24cc7397a", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			InputTenant: "acme",
			OutputError: nil,
		},
	}

	for testCaseNumber, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %d", testCaseNumber+1), func(t *testing.T) {

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if testCase.InputTenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: testCase.InputTenant,
				})
			}

			err := store.InsertMany(ctx,
				testCase.InputDeviceDeployment...)

			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)

				count, err := session.DB(ctxstore.DbFromContext(ctx, DatabaseName)).
					C(CollectionDevices).
					Find(nil).Count()
				assert.NoError(t, err)
				assert.Equal(t, len(testCase.InputDeviceDeployment), count)

				if testCase.InputTenant != "" {
					// deployment was added to tenant's DB,
					// make sure it's not in default DB
					count, err := session.DB(DatabaseName).
						C(CollectionDevices).
						Find(nil).Count()
					assert.NoError(t, err)
					assert.Equal(t, 0, count)
				}
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

func TestUpdateDeviceDeploymentStatus(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestUpdateDeviceDeploymentStatus in short mode.")
	}

	now := time.Now()

	testCases := []struct {
		InputDeviceID         string
		InputDeploymentID     string
		InputStatus           string
		InputSubState         *string
		InputDeviceDeployment []*deployments.DeviceDeployment
		InputFinishTime       *time.Time
		InputTenant           string

		OutputError     error
		OutputOldStatus string
	}{
		{
			// null status
			InputDeviceID:     "123",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			OutputError:       ErrStorageInvalidInput,
			OutputOldStatus:   "",
		},
		{
			// null deployment ID
			InputDeviceID:   "234",
			InputStatus:     "",
			OutputError:     ErrStorageInvalidID,
			OutputOldStatus: "",
		},
		{
			// null device ID
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputStatus:       "notnull",
			OutputError:       ErrStorageInvalidID,
			OutputOldStatus:   "",
		},
		{
			// no deployment/device with this ID
			InputDeviceID:     "345",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputStatus:       "notnull",
			OutputError:       ErrStorageNotFound,
			OutputOldStatus:   "",
		},
		{
			InputDeviceID:     "456",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputStatus:       deployments.DeviceDeploymentStatusInstalling,
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			OutputError:     nil,
			OutputOldStatus: "pending",
		},
		{
			InputDeviceID:     "567",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputStatus:       deployments.DeviceDeploymentStatusFailure,
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("567", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			InputFinishTime: &now,
			OutputError:     nil,
			OutputOldStatus: "pending",
		},
		{
			InputDeviceID:     "678",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397d",
			InputStatus:       deployments.DeviceDeploymentStatusInstalling,
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("678", "30b3e62c-9ec2-4312-a7fa-cff24cc7397d"),
			},
			InputTenant:     "acme",
			OutputOldStatus: "pending",
		},
		{
			InputDeviceID:     "12345",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397e",
			InputStatus:       deployments.DeviceDeploymentStatusInstalling,
			InputSubState:     pointers.StringToPointer("foobar 123"),
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("12345", "30b3e62c-9ec2-4312-a7fa-cff24cc7397e"),
			},
			OutputError:     nil,
			OutputOldStatus: "pending",
		},
	}

	for testCaseNumber, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %d", testCaseNumber+1), func(t *testing.T) {

			t.Logf("testing case %s %s %s %v",
				testCase.InputDeviceID, testCase.InputDeploymentID,
				testCase.InputStatus, testCase.OutputError)

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if testCase.InputTenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: testCase.InputTenant,
				})
			}

			// deployments are created with status DeviceDeploymentStatusPending
			err := store.InsertMany(ctx, testCase.InputDeviceDeployment...)
			assert.NoError(t, err)

			old, err := store.UpdateDeviceDeploymentStatus(ctx,
				testCase.InputDeviceID, testCase.InputDeploymentID,
				deployments.DeviceDeploymentStatus{
					Status:     testCase.InputStatus,
					SubState:   testCase.InputSubState,
					FinishTime: testCase.InputFinishTime,
				})

			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)

				if testCase.InputTenant != "" {
					// update in tenant's DB was successful,
					// similar update in default DB should
					// fail because deployments are present
					// in tenant's DB only
					_, err := store.UpdateDeviceDeploymentStatus(context.Background(),
						testCase.InputDeviceID, testCase.InputDeploymentID,
						deployments.DeviceDeploymentStatus{
							Status:     testCase.InputStatus,
							FinishTime: testCase.InputFinishTime,
							SubState:   testCase.InputSubState,
						})
					t.Logf("error: %+v", err)
					assert.EqualError(t, err, ErrStorageNotFound.Error())
				}
			}

			if testCase.InputDeviceDeployment != nil {
				// these checks only make sense if there are any deployments in database
				var deployment *deployments.DeviceDeployment
				dep := session.DB(ctxstore.DbFromContext(ctx, DatabaseName)).C(CollectionDevices)
				query := bson.M{
					StorageKeyDeviceDeploymentDeviceId:     testCase.InputDeviceID,
					StorageKeyDeviceDeploymentDeploymentID: testCase.InputDeploymentID,
				}
				err := dep.Find(query).One(&deployment)
				assert.NoError(t, err)
				if testCase.OutputError != nil {
					// status must be unchanged in case of errors
					assert.Equal(t, deployments.DeviceDeploymentStatusPending, deployment.Status)
				} else {
					if !assert.NotNil(t, deployment) {
						return
					}

					assert.Equal(t, testCase.InputStatus, *deployment.Status)
					assert.Equal(t, testCase.OutputOldStatus, old)
					// verify deployment finish time
					if testCase.InputFinishTime != nil && assert.NotNil(t, deployment.Finished) {
						// mongo might have trimmed our
						// time a bit, let's check that
						// we are within a 1s range
						assert.WithinDuration(t, *testCase.InputFinishTime,
							*deployment.Finished, time.Second)
					}

					if testCase.InputSubState != nil {
						assert.Equal(t, *testCase.InputSubState, *deployment.SubState)
					}
				}
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

func TestUpdateDeviceDeploymentLogAvailability(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestUpdateDeviceDeploymentLogAvailability in short mode.")
	}

	testCases := []struct {
		InputDeviceID         string
		InputDeploymentID     string
		InputLog              bool
		InputDeviceDeployment []*deployments.DeviceDeployment
		InputTenant           string

		OutputError error
	}{
		{
			// null deployment ID
			InputDeviceID: "234",
			OutputError:   ErrStorageInvalidID,
		},
		{
			// null device ID
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			OutputError:       ErrStorageInvalidID,
		},
		{
			// no deployment/device with this ID
			InputDeviceID:     "345",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputLog:          true,
			OutputError:       ErrStorageNotFound,
		},
		{
			InputDeviceID:     "456",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputLog:          true,
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			OutputError: nil,
		},
		{
			InputDeviceID:     "456",
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputLog:          false,
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			InputTenant: "acme",
		},
	}

	for testCaseNumber, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %d", testCaseNumber+1), func(t *testing.T) {

			t.Logf("testing case %s %s %t %v",
				testCase.InputDeviceID, testCase.InputDeploymentID,
				testCase.InputLog, testCase.OutputError)

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if testCase.InputTenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: testCase.InputTenant,
				})
			}

			// deployments are created with status DeviceDeploymentStatusPending
			err := store.InsertMany(ctx, testCase.InputDeviceDeployment...)
			assert.NoError(t, err)

			err = store.UpdateDeviceDeploymentLogAvailability(ctx,
				testCase.InputDeviceID, testCase.InputDeploymentID, testCase.InputLog)

			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)

				if testCase.InputTenant != "" {
					// we're using tenant's DB, so acting on default DB should fail
					err := store.UpdateDeviceDeploymentLogAvailability(context.Background(),
						testCase.InputDeviceID, testCase.InputDeploymentID,
						testCase.InputLog)
					assert.EqualError(t, err, ErrStorageNotFound.Error())
				}
			}

			if testCase.InputDeviceDeployment != nil {
				var deployment *deployments.DeviceDeployment
				query := bson.M{
					StorageKeyDeviceDeploymentDeviceId:     testCase.InputDeviceID,
					StorageKeyDeviceDeploymentDeploymentID: testCase.InputDeploymentID,
				}
				err := session.DB(ctxstore.DbFromContext(ctx, DatabaseName)).
					C(CollectionDevices).
					Find(query).One(&deployment)

				assert.NoError(t, err)
				assert.Equal(t, testCase.InputLog, deployment.IsLogAvailable)
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

func newDeviceDeploymentWithStatus(deviceID string, deploymentID string, status string) *deployments.DeviceDeployment {
	d := deployments.NewDeviceDeployment(deviceID, deploymentID)
	d.Status = &status
	return d
}

func TestAggregateDeviceDeploymentByStatus(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestAggregateDeviceDeploymentByStatus in short mode.")
	}

	testCases := []struct {
		InputDeploymentID     string
		InputDeviceDeployment []*deployments.DeviceDeployment
		InputTenant           string
		OutputError           error
		OutputStats           deployments.Stats
	}{
		{
			InputDeploymentID:     "ee13ea8b-a6d3-4d4c-99a6-bcfcaebc7ec3",
			InputDeviceDeployment: nil,
			OutputError:           nil,
			OutputStats:           nil,
		},
		{
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				newDeviceDeploymentWithStatus("123", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusFailure),
				newDeviceDeploymentWithStatus("234", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusFailure),
				newDeviceDeploymentWithStatus("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusSuccess),

				// these 2 count as in progress
				newDeviceDeploymentWithStatus("567", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusDownloading),
				newDeviceDeploymentWithStatus("678", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusRebooting),

				newDeviceDeploymentWithStatus("789", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusPending),
			},
			OutputError: nil,
			OutputStats: deployments.Stats{
				deployments.DeviceDeploymentStatusPending:        1,
				deployments.DeviceDeploymentStatusSuccess:        1,
				deployments.DeviceDeploymentStatusFailure:        2,
				deployments.DeviceDeploymentStatusRebooting:      1,
				deployments.DeviceDeploymentStatusDownloading:    1,
				deployments.DeviceDeploymentStatusInstalling:     0,
				deployments.DeviceDeploymentStatusNoArtifact:     0,
				deployments.DeviceDeploymentStatusAlreadyInst:    0,
				deployments.DeviceDeploymentStatusAborted:        0,
				deployments.DeviceDeploymentStatusDecommissioned: 0,
			},
		},
		{
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				newDeviceDeploymentWithStatus("123", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusFailure),
				newDeviceDeploymentWithStatus("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
					deployments.DeviceDeploymentStatusSuccess),
			},
			InputTenant: "acme",
			OutputStats: newTestStats(deployments.Stats{
				deployments.DeviceDeploymentStatusSuccess: 1,
				deployments.DeviceDeploymentStatusFailure: 1,
			}),
		},
	}

	for testCaseNumber, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %d", testCaseNumber+1), func(t *testing.T) {

			t.Logf("testing case %s %v %d", testCase.InputDeploymentID, testCase.OutputError,
				len(testCase.InputDeviceDeployment))

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if testCase.InputTenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: testCase.InputTenant,
				})
			}

			err := store.InsertMany(ctx, testCase.InputDeviceDeployment...)
			assert.NoError(t, err)

			stats, err := store.AggregateDeviceDeploymentByStatus(ctx,
				testCase.InputDeploymentID)
			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)

				if testCase.InputTenant != "" {
					// data was inserted into tenant's DB,
					// verify that aggregates are all 0
					stats, err := store.AggregateDeviceDeploymentByStatus(context.Background(),
						testCase.InputDeploymentID)
					assert.NoError(t, err)
					assert.Equal(t, newTestStats(deployments.Stats{}), stats)
				}
			}

			if testCase.OutputStats != nil {
				assert.NotNil(t, stats)
				assert.Equal(t, testCase.OutputStats, stats)
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

func TestGetDeviceStatusesForDeployment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GetDeviceStatusesForDeployment in short mode.")
	}

	input := []*deployments.DeviceDeployment{
		deployments.NewDeviceDeployment("device0001", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0002", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0003", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0004", "30b3e62c-9ec2-4312-a7fa-cff24cc7397b"),
		deployments.NewDeviceDeployment("device0005", "30b3e62c-9ec2-4312-a7fa-cff24cc7397b"),
	}

	testCases := map[string]struct {
		caseId string
		tenant string

		inputDeploymentId string
		outputStatuses    []*deployments.DeviceDeployment
	}{
		"existing deployments 1": {
			inputDeploymentId: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			outputStatuses:    input[:3],
		},
		"existing deployments 2": {
			inputDeploymentId: "30b3e62c-9ec2-4312-a7fa-cff24cc7397b",
			outputStatuses:    input[3:],
		},
		"nonexistent deployment": {
			inputDeploymentId: "aaaaaaaa-9ec2-4312-a7fa-cff24cc7397b",
			outputStatuses:    []*deployments.DeviceDeployment{},
		},
		"tenant, existing deployments": {
			inputDeploymentId: "30b3e62c-9ec2-4312-a7fa-cff24cc7397b",
			tenant:            "acme",
			outputStatuses:    input[3:],
		},
	}

	for testCaseName, tc := range testCases {
		t.Run(fmt.Sprintf("test case %s", testCaseName), func(t *testing.T) {

			// setup db - once for all cases
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if tc.tenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: tc.tenant,
				})
			}

			err := store.InsertMany(ctx, input...)
			assert.NoError(t, err)

			statuses, err := store.GetDeviceStatusesForDeployment(ctx,
				tc.inputDeploymentId)
			assert.NoError(t, err)

			assert.Equal(t, len(tc.outputStatuses), len(statuses))
			for i, out := range tc.outputStatuses {
				assert.Equal(t, out.DeviceId, statuses[i].DeviceId)
				assert.Equal(t, out.DeploymentId, statuses[i].DeploymentId)
			}

			if tc.tenant != "" {
				// deployment statuses are present in tenant's
				// DB, verify that listing from default DB
				// yields empty list
				statuses, err := store.GetDeviceStatusesForDeployment(context.Background(),
					tc.inputDeploymentId)
				assert.NoError(t, err)
				assert.Len(t, statuses, 0)
			}

			session.Close()
		})
	}
}

func TestHasDeploymentForDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GetDeviceStatusesForDeployment in short mode.")
	}

	input := []*deployments.DeviceDeployment{
		deployments.NewDeviceDeployment("device0001", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0002", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0003", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
	}

	testCases := []struct {
		deviceID     string
		deploymentID string
		tenant       string

		has bool
		err error
	}{
		{
			deviceID:     "device0001",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			has:          true,
			err:          nil,
		},
		{
			deviceID:     "device0002",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			has:          true,
			err:          nil,
		},
		{
			deviceID:     "device0003",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397b",
			has:          false,
		},
		{
			deviceID:     "device0004",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397c",
			has:          false,
		},
		{
			deviceID:     "device0003",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			has:          true,
			tenant:       "acme",
		},
	}

	for testCaseNumber, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", testCaseNumber+1), func(t *testing.T) {

			t.Logf("testing case: %v %v %v %v", tc.deviceID, tc.deploymentID, tc.has, tc.err)

			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if tc.tenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: tc.tenant,
				})
			}

			err := store.InsertMany(ctx, input...)
			assert.NoError(t, err)

			has, err := store.HasDeploymentForDevice(ctx,
				tc.deploymentID, tc.deviceID)
			if tc.err != nil {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.err.Error())
				assert.False(t, has)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.has, has)

				if tc.tenant != "" {
					// data was added to tenant's DB, verify
					// that there's no deployment if looking
					// in default DB
					has, err := store.HasDeploymentForDevice(context.Background(),
						tc.deploymentID, tc.deviceID)
					assert.False(t, has)
					assert.NoError(t, err)
				}
			}

			session.Close()
		})
	}
}

func TestGetDeviceDeploymentStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GetDeviceDeploymentStatus in short mode.")
	}

	input := []*deployments.DeviceDeployment{
		deployments.NewDeviceDeployment("device0001", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0002", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
		deployments.NewDeviceDeployment("device0003", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
	}

	testCases := map[string]struct {
		deviceID     string
		deploymentID string
		tenant       string

		status string
	}{
		"device deployment exists": {
			deviceID:     "device0001",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			status:       "pending",
		},
		"deployment not exists": {
			deviceID:     "device0003",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397b",
			status:       "",
		},
		"no deployment for device": {
			deviceID:     "device0004",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397c",
			status:       "",
		},
		"tenant, device deployment exists": {
			deviceID:     "device0001",
			deploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			status:       "pending",
			tenant:       "acme",
		},
	}

	for testCaseName, tc := range testCases {
		t.Run(fmt.Sprintf("test case %s", testCaseName), func(t *testing.T) {

			t.Logf("testing case: %v %v %v", tc.deviceID, tc.deploymentID, tc.status)

			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			ctx := context.Background()
			if tc.tenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: tc.tenant,
				})
			}

			err := store.InsertMany(ctx, input...)
			assert.NoError(t, err)

			status, err := store.GetDeviceDeploymentStatus(ctx,
				tc.deploymentID, tc.deviceID)
			assert.NoError(t, err)
			assert.Equal(t, tc.status, status)

			if tc.tenant != "" {
				// data was added to tenant's DB, trying to
				// fetch it from default DB will not fail but
				// returns empty status instead
				status, err := store.GetDeviceDeploymentStatus(context.Background(),
					tc.deploymentID, tc.deviceID)
				assert.NoError(t, err)
				assert.Equal(t, "", status)
			}

			session.Close()
		})
	}

}

func TestAbortDeviceDeployments(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestAbortDeviceDeployments in short mode.")
	}

	testCases := map[string]struct {
		InputDeploymentID     string
		InputDeviceDeployment []*deployments.DeviceDeployment

		OutputError error
	}{
		"null deployment id": {
			OutputError: ErrStorageInvalidID,
		},
		"all correct": {
			InputDeploymentID: "30b3e62c-9ec2-4312-a7fa-cff24cc7397a",
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("456", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
				deployments.NewDeviceDeployment("567", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			OutputError: nil,
		},
	}

	for testCaseName, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %s", testCaseName), func(t *testing.T) {

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			err := store.InsertMany(context.Background(), testCase.InputDeviceDeployment...)
			assert.NoError(t, err)

			err = store.AbortDeviceDeployments(context.Background(), testCase.InputDeploymentID)

			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)
			}

			if testCase.InputDeviceDeployment != nil {
				// these checks only make sense if there are any deployments in database
				var deploymentList []deployments.DeviceDeployment
				dep := session.DB(DatabaseName).C(CollectionDevices)
				query := bson.M{
					StorageKeyDeviceDeploymentDeploymentID: testCase.InputDeploymentID,
				}
				err := dep.Find(query).All(&deploymentList)
				assert.NoError(t, err)

				if testCase.OutputError != nil {
					for _, deployment := range deploymentList {
						// status must be unchanged in case of errors
						assert.Equal(t, deployments.DeviceDeploymentStatusPending,
							*deployment.Status)
					}
				} else {
					for _, deployment := range deploymentList {
						assert.Equal(t, deployments.DeviceDeploymentStatusAborted,
							*deployment.Status)
					}
				}
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

func TestDecommissionDeviceDeployments(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping TestDecommissionDeviceDeployments in short mode.")
	}

	testCases := map[string]struct {
		InputDeviceId         string
		InputDeviceDeployment []*deployments.DeviceDeployment

		OutputError error
	}{
		"null device id": {
			OutputError: ErrStorageInvalidID,
		},
		"all correct": {
			InputDeviceId: "foo",
			InputDeviceDeployment: []*deployments.DeviceDeployment{
				deployments.NewDeviceDeployment("foo", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
				deployments.NewDeviceDeployment("bar", "30b3e62c-9ec2-4312-a7fa-cff24cc7397a"),
			},
			OutputError: nil,
		},
	}

	for testCaseName, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %s", testCaseName), func(t *testing.T) {

			// Make sure we start test with empty database
			db.Wipe()

			session := db.Session()
			store := NewDeviceDeploymentsStorage(session)

			err := store.InsertMany(context.Background(), testCase.InputDeviceDeployment...)
			assert.NoError(t, err)

			err = store.DecommissionDeviceDeployments(context.Background(), testCase.InputDeviceId)

			if testCase.OutputError != nil {
				assert.EqualError(t, err, testCase.OutputError.Error())
			} else {
				assert.NoError(t, err)
			}

			if testCase.InputDeviceDeployment != nil {
				// these checks only make sense if there are any deployments in database
				var deploymentList []deployments.DeviceDeployment
				dep := session.DB(DatabaseName).C(CollectionDevices)
				query := bson.M{
					StorageKeyDeviceDeploymentDeviceId: testCase.InputDeviceId,
				}
				err := dep.Find(query).All(&deploymentList)
				assert.NoError(t, err)

				if testCase.OutputError != nil {
					for _, deployment := range deploymentList {
						// status must be unchanged in case of errors
						assert.Equal(t, deployments.DeviceDeploymentStatusPending,
							*deployment.Status)
					}
				} else {
					for _, deployment := range deploymentList {
						assert.Equal(t, deployments.DeviceDeploymentStatusDecommissioned,
							*deployment.Status)
					}
				}
			}

			// Need to close all sessions to be able to call wipe at next test case
			session.Close()
		})
	}
}

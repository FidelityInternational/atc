package db_test

import (
	"time"

	"github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
)

var _ = Describe("Keeping track of containers", func() {
	var dbConn db.Conn
	var listener *pq.Listener

	var database *db.SQLDB
	var savedPipeline db.SavedPipeline
	var pipelineDB db.PipelineDB

	BeforeEach(func() {
		var err error

		postgresRunner.Truncate()

		dbConn = db.Wrap(postgresRunner.Open())

		listener = pq.NewListener(postgresRunner.DataSourceName(), time.Second, time.Minute, nil)

		Eventually(listener.Ping, 5*time.Second).ShouldNot(HaveOccurred())
		bus := db.NewNotificationsBus(listener, dbConn)

		database = db.NewSQL(lagertest.NewTestLogger("test"), dbConn, bus)

		config := atc.Config{
			Jobs: atc.JobConfigs{
				{
					Name: "some-job",
				},
				{
					Name: "some-other-job",
				},
				{
					Name: "some-random-job",
				},
			},
			Resources: atc.ResourceConfigs{
				{
					Name: "some-resource",
					Type: "some-type",
				},
				{
					Name: "some-other-resource",
					Type: "some-other-type",
				},
			},
		}

		savedPipeline, _, err = database.SaveConfig(atc.DefaultTeamName, "some-pipeline", config, 0, db.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = database.SaveConfig(atc.DefaultTeamName, "some-other-pipeline", config, 0, db.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		pipelineDBFactory := db.NewPipelineDBFactory(nil, dbConn, nil, database)
		pipelineDB = pipelineDBFactory.Build(savedPipeline)
	})

	AfterEach(func() {
		err := dbConn.Close()
		Expect(err).NotTo(HaveOccurred())

		err = listener.Close()
		Expect(err).NotTo(HaveOccurred())
	})

	getResourceID := func(name string) int {
		savedResource, err := pipelineDB.GetResource(name)
		Expect(err).NotTo(HaveOccurred())
		return savedResource.ID
	}

	getJobBuildID := func(jobName string) int {
		savedBuild, err := pipelineDB.CreateJobBuild(jobName)
		Expect(err).NotTo(HaveOccurred())
		return savedBuild.ID
	}

	getOneOffBuildID := func() int {
		savedBuild, err := database.CreateOneOffBuild()
		Expect(err).NotTo(HaveOccurred())
		return savedBuild.ID
	}

	It("can create and get a resource container object", func() {
		containerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				ResourceID:  getResourceID("some-resource"),
				CheckType:   "some-resource-type",
				CheckSource: atc.Source{"some": "source"},
				Stage:       db.ContainerStageRun,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:               "some-handle",
				WorkerName:           "some-worker",
				PipelineName:         "some-pipeline",
				Type:                 db.ContainerTypeCheck,
				WorkingDirectory:     "tmp/build/some-guid",
				EnvironmentVariables: []string{"VAR1=val1", "VAR2=val2"},
			},
		}

		By("creating a container")
		_, err := database.CreateContainer(containerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		By("trying to create a container with the same handle")
		matchingHandleContainer := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage: db.ContainerStageRun,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle: "some-handle",
			},
		}
		_, err = database.CreateContainer(matchingHandleContainer, time.Second)
		Expect(err).To(HaveOccurred())

		By("getting the saved info object by handle")
		actualContainer, found, err := database.GetContainer("some-handle")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		Expect(actualContainer.WorkerName).To(Equal(containerToCreate.WorkerName))
		Expect(actualContainer.ResourceID).To(Equal(containerToCreate.ResourceID))

		Expect(actualContainer.Handle).To(Equal(containerToCreate.Handle))
		Expect(actualContainer.StepName).To(Equal(""))
		Expect(actualContainer.ResourceName).To(Equal("some-resource"))
		Expect(actualContainer.PipelineID).To(Equal(savedPipeline.ID))
		Expect(actualContainer.PipelineName).To(Equal(savedPipeline.Name))
		Expect(actualContainer.BuildID).To(Equal(0))
		Expect(actualContainer.Type).To(Equal(db.ContainerTypeCheck))
		Expect(actualContainer.ContainerMetadata.WorkerName).To(Equal(containerToCreate.WorkerName))
		Expect(actualContainer.WorkingDirectory).To(Equal(containerToCreate.WorkingDirectory))
		Expect(actualContainer.CheckType).To(Equal(containerToCreate.CheckType))
		Expect(actualContainer.CheckSource).To(Equal(containerToCreate.CheckSource))
		Expect(actualContainer.EnvironmentVariables).To(Equal(containerToCreate.EnvironmentVariables))

		By("returning found = false when getting by a handle that does not exist")
		_, found, err = database.GetContainer("nope")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("can create and get a step container info object", func() {
		containerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				BuildID: 1111,
				PlanID:  "some-plan-id",
				Stage:   db.ContainerStageRun,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:               "some-handle",
				WorkerName:           "some-worker",
				PipelineName:         "some-pipeline",
				StepName:             "some-step-container",
				Type:                 db.ContainerTypeTask,
				WorkingDirectory:     "tmp/build/some-guid",
				EnvironmentVariables: []string{"VAR1=val1", "VAR2=val2"},
				Attempts:             []int{1, 2, 4},
			},
		}

		By("creating a container")
		_, err := database.CreateContainer(containerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		By("trying to create a container with the same handle")
		_, err = database.CreateContainer(db.Container{ContainerMetadata: db.ContainerMetadata{Handle: "some-handle"}}, time.Second)
		Expect(err).To(HaveOccurred())

		By("getting the saved info object by handle")
		actualContainer, found, err := database.GetContainer("some-handle")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		Expect(actualContainer.BuildID).To(Equal(containerToCreate.BuildID))
		Expect(actualContainer.PlanID).To(Equal(containerToCreate.PlanID))

		Expect(actualContainer.Handle).To(Equal(containerToCreate.Handle))
		Expect(actualContainer.WorkerName).To(Equal(containerToCreate.WorkerName))
		Expect(actualContainer.PipelineID).To(Equal(savedPipeline.ID))
		Expect(actualContainer.PipelineName).To(Equal(containerToCreate.PipelineName))
		Expect(actualContainer.StepName).To(Equal(containerToCreate.StepName))
		Expect(actualContainer.Type).To(Equal(containerToCreate.Type))
		Expect(actualContainer.WorkingDirectory).To(Equal(containerToCreate.WorkingDirectory))
		Expect(actualContainer.EnvironmentVariables).To(Equal(containerToCreate.EnvironmentVariables))
		Expect(actualContainer.Attempts).To(Equal(containerToCreate.Attempts))

		Expect(actualContainer.ResourceID).To(Equal(0))
		Expect(actualContainer.ResourceName).To(Equal(""))
		Expect(actualContainer.CheckType).To(BeEmpty())
		Expect(actualContainer.CheckSource).To(BeEmpty())

		By("returning found = false when getting by a handle that does not exist")
		_, found, err = database.GetContainer("nope")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("can update the time to live for a container info object", func() {
		containerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage: db.ContainerStageRun,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       "some-handle",
				Type:         db.ContainerTypeTask,
				WorkerName:   "some-worker",
				PipelineName: "some-pipeline",
			},
		}

		_, err := database.CreateContainer(containerToCreate, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred())

		timeBefore := time.Now()
		err = database.UpdateExpiresAtOnContainer("some-handle", time.Second)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			_, found, err := database.GetContainer("some-handle")
			Expect(err).NotTo(HaveOccurred())
			return found
		}, 10*time.Second).Should(BeFalse())

		timeAfter := time.Now()
		Expect(timeAfter.Sub(timeBefore)).To(BeNumerically(">=", time.Second))
		Expect(timeAfter.Sub(timeBefore)).To(BeNumerically("<", 10*time.Second))
	})

	It("can reap a container", func() {
		containerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage: db.ContainerStageRun,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       "some-reaped-handle",
				Type:         db.ContainerTypeTask,
				WorkerName:   "some-worker",
				PipelineName: "some-pipeline",
			},
		}
		_, err := database.CreateContainer(containerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		_, found, err := database.GetContainer("some-reaped-handle")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		By("reaping an existing container")
		err = database.ReapContainer("some-reaped-handle")
		Expect(err).NotTo(HaveOccurred())

		_, found, err = database.GetContainer("some-reaped-handle")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())

		By("not failing if the container's already been reaped")
		err = database.ReapContainer("some-reaped-handle")
		Expect(err).NotTo(HaveOccurred())
	})

	It("differentiates between containers with different stages", func() {
		someBuild, err := database.CreateOneOffBuild()
		Expect(err).ToNot(HaveOccurred())

		checkStageContainerID := db.ContainerIdentifier{
			BuildID: someBuild.ID,
			PlanID:  atc.PlanID("some-task"),
			Stage:   db.ContainerStageCheck,
		}

		getStageContainerID := db.ContainerIdentifier{
			BuildID: someBuild.ID,
			PlanID:  atc.PlanID("some-task"),
			Stage:   db.ContainerStageGet,
		}

		runStageContainerID := db.ContainerIdentifier{
			BuildID: someBuild.ID,
			PlanID:  atc.PlanID("some-task"),
			Stage:   db.ContainerStageRun,
		}

		checkContainer, err := database.CreateContainer(db.Container{
			ContainerIdentifier: checkStageContainerID,
			ContainerMetadata: db.ContainerMetadata{
				Handle: "check-handle",
				Type:   db.ContainerTypeCheck,
			},
		}, time.Minute)
		Expect(err).ToNot(HaveOccurred())

		getContainer, err := database.CreateContainer(db.Container{
			ContainerIdentifier: getStageContainerID,
			ContainerMetadata: db.ContainerMetadata{
				Handle: "get-handle",
				Type:   db.ContainerTypeGet,
			},
		}, time.Minute)
		Expect(err).ToNot(HaveOccurred())

		runContainer, err := database.CreateContainer(db.Container{
			ContainerIdentifier: runStageContainerID,
			ContainerMetadata: db.ContainerMetadata{
				Handle: "run-handle",
				Type:   db.ContainerTypeTask,
			},
		}, time.Minute)
		Expect(err).ToNot(HaveOccurred())

		container, found, err := database.FindContainerByIdentifier(checkStageContainerID)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(container.ContainerIdentifier).To(Equal(checkContainer.ContainerIdentifier))

		container, found, err = database.FindContainerByIdentifier(getStageContainerID)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(container.ContainerIdentifier).To(Equal(getContainer.ContainerIdentifier))

		container, found, err = database.FindContainerByIdentifier(runStageContainerID)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(container.ContainerIdentifier).To(Equal(runContainer.ContainerIdentifier))
	})

	type findContainersByDescriptorsExample struct {
		containersToCreate     []db.Container
		descriptorsToFilterFor db.Container
		expectedHandles        []string
	}

	DescribeTable("filtering containers by metadata",
		func(exampleGenerator func() findContainersByDescriptorsExample) {
			var results []db.Container
			var handles []string
			var err error

			example := exampleGenerator()

			for _, containerToCreate := range example.containersToCreate {
				if containerToCreate.Type.String() == "" {
					containerToCreate.Type = db.ContainerTypeTask
				}

				_, err := database.CreateContainer(containerToCreate, time.Minute)
				Expect(err).NotTo(HaveOccurred())
			}

			results, err = database.FindContainersByDescriptors(example.descriptorsToFilterFor)
			Expect(err).NotTo(HaveOccurred())

			for _, result := range results {
				handles = append(handles, result.Handle)
			}

			Expect(handles).To(ConsistOf(example.expectedHandles))

			for _, containerToDelete := range example.containersToCreate {
				err = database.DeleteContainer(containerToDelete.Handle)
				Expect(err).NotTo(HaveOccurred())
			}
		},

		Entry("returns everything when no filters are passed", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{}},
				expectedHandles:        []string{"a", "b"},
			}
		}),

		Entry("does not return things that the filter doesn't match", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{ResourceName: "some-resource"}},
				expectedHandles:        nil,
			}
		}),

		Entry("returns containers where the step name matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							StepName:     "some-step",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
							StepName:     "some-other-step",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
							StepName:     "some-step",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{StepName: "some-step"}},
				expectedHandles:        []string{"a", "c"},
			}
		}),

		Entry("returns containers where the resource name matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							ResourceID: getResourceID("some-resource"),
							Stage:      db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							ResourceID: getResourceID("some-resource"),
							Stage:      db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							ResourceID: getResourceID("some-other-resource"),
							Stage:      db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
							ResourceName: "some-other-resource",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{ResourceName: "some-resource"}},
				expectedHandles:        []string{"a", "b"},
			}
		}),

		Entry("returns containers where the pipeline matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-Oother-worker",
							PipelineName: "some-pipeline",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{PipelineName: "some-pipeline"}},
				expectedHandles:        []string{"a", "c"},
			}
		}),

		Entry("returns containers where the type matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypePut,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypePut,
							WorkerName:   "some-other-worker",
							PipelineName: "some-other-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeGet,
							WorkerName:   "some-Oother-worker",
							PipelineName: "some-pipeline",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{Type: db.ContainerTypePut}},
				expectedHandles:        []string{"a", "b"},
			}
		}),

		Entry("returns containers where the worker name matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypePut,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypePut,
							WorkerName:   "some-worker",
							PipelineName: "some-other-pipeline",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeGet,
							WorkerName:   "some-other-worker",
							PipelineName: "some-pipeline",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{WorkerName: "some-worker"}},
				expectedHandles:        []string{"a", "b"},
			}
		}),

		Entry("returns containers where the check type matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage:     db.ContainerStageRun,
							CheckType: "",
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage:     db.ContainerStageRun,
							CheckType: "nope",
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-worker",
							PipelineName: "some-other-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage:     db.ContainerStageRun,
							CheckType: "some-type",
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-other-worker",
							PipelineName: "some-pipeline",
							ResourceName: "some-resource",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerIdentifier: db.ContainerIdentifier{CheckType: "some-type"}},
				expectedHandles:        []string{"c"},
			}
		}),

		Entry("returns containers where the check source matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
							CheckSource: atc.Source{
								"some": "other-source",
							},
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "a",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "b",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-worker",
							PipelineName: "some-other-pipeline",
							ResourceName: "some-resource",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
							CheckSource: atc.Source{
								"some": "source",
							},
						},
						ContainerMetadata: db.ContainerMetadata{
							Handle:       "c",
							Type:         db.ContainerTypeCheck,
							WorkerName:   "some-other-worker",
							PipelineName: "some-pipeline",
							ResourceName: "some-resource",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerIdentifier: db.ContainerIdentifier{CheckSource: atc.Source{"some": "source"}}},
				expectedHandles:        []string{"c"},
			}
		}),

		Entry("returns containers where the job name matches", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{{
					ContainerIdentifier: db.ContainerIdentifier{
						Stage:   db.ContainerStageRun,
						BuildID: getJobBuildID("some-other-job"),
					},
					ContainerMetadata: db.ContainerMetadata{
						Type:         db.ContainerTypeTask,
						WorkerName:   "some-worker",
						PipelineName: "some-pipeline",
						JobName:      "some-other-job",
						Handle:       "a",
					},
				},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage:   db.ContainerStageRun,
							BuildID: getJobBuildID("some-job"),
						},
						ContainerMetadata: db.ContainerMetadata{
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							JobName:      "some-job",
							Handle:       "b",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage:   db.ContainerStageRun,
							BuildID: getOneOffBuildID(),
						},
						ContainerMetadata: db.ContainerMetadata{
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "",
							JobName:      "",
							Handle:       "c",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{JobName: "some-job"}},
				expectedHandles:        []string{"b"},
			}
		}),

		Entry("returns containers where the attempts numbers match", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{{
					ContainerIdentifier: db.ContainerIdentifier{
						Stage: db.ContainerStageRun,
					},
					ContainerMetadata: db.ContainerMetadata{
						Type:         db.ContainerTypeTask,
						WorkerName:   "some-worker",
						PipelineName: "some-pipeline",
						Attempts:     []int{1, 2, 5},
						Handle:       "a",
					},
				},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							Attempts:     []int{1, 2},
							Handle:       "b",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-other-worker",
							PipelineName: "some-pipeline",
							Attempts:     []int{1},
							Handle:       "c",
						},
					},
				},
				descriptorsToFilterFor: db.Container{ContainerMetadata: db.ContainerMetadata{Attempts: []int{1, 2}}},
				expectedHandles:        []string{"b"},
			}
		}),

		Entry("returns containers where all fields match", func() findContainersByDescriptorsExample {
			return findContainersByDescriptorsExample{
				containersToCreate: []db.Container{
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							StepName:     "some-name",
							PipelineName: "some-pipeline",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							Handle:       "a",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							StepName:     "WROONG",
							PipelineName: "some-pipeline",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							Handle:       "b",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							StepName:     "some-name",
							PipelineName: "some-pipeline",
							Type:         db.ContainerTypeTask,
							WorkerName:   "some-worker",
							Handle:       "c",
						},
					},
					{
						ContainerIdentifier: db.ContainerIdentifier{
							Stage: db.ContainerStageRun,
						},
						ContainerMetadata: db.ContainerMetadata{
							WorkerName:   "some-worker",
							PipelineName: "some-pipeline",
							Type:         db.ContainerTypeTask,
							Handle:       "d",
						},
					},
				},
				descriptorsToFilterFor: db.Container{
					ContainerMetadata: db.ContainerMetadata{
						StepName:     "some-name",
						PipelineName: "some-pipeline",
						Type:         db.ContainerTypeTask,
						WorkerName:   "some-worker",
					},
				},
				expectedHandles: []string{"a", "c"},
			}
		}),
	)

	It("can find a single container info by identifier", func() {
		handle := "some-handle"
		otherHandle := "other-handle"

		containerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage:       db.ContainerStageRun,
				CheckType:   "some-type",
				CheckSource: atc.Source{"some": "other-source"},
				ResourceID:  getResourceID("some-resource"),
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       handle,
				PipelineName: "some-pipeline",
				ResourceName: "some-resource",
				WorkerName:   "some-worker",
				Type:         db.ContainerTypeCheck,
			},
		}
		stepContainerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage:   db.ContainerStageRun,
				PlanID:  atc.PlanID("plan-id"),
				BuildID: 555,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       otherHandle,
				PipelineName: "some-pipeline",
				WorkerName:   "some-worker",
				StepName:     "other-container",
				Type:         db.ContainerTypeTask,
			},
		}
		otherStepContainer := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage:   db.ContainerStageRun,
				PlanID:  atc.PlanID("other-plan-id"),
				BuildID: 666,
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       "very-other-handle",
				PipelineName: "some-pipeline",
				WorkerName:   "some-worker",
				StepName:     "other-container",
				Type:         db.ContainerTypeTask,
			},
		}

		_, err := database.CreateContainer(containerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())
		_, err = database.CreateContainer(stepContainerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())
		_, err = database.CreateContainer(otherStepContainer, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		all_containers := getAllContainers(dbConn)
		Expect(all_containers).To(HaveLen(3))

		By("returning a single matching resource container info")
		actualContainer, found, err := database.FindContainerByIdentifier(
			containerToCreate.ContainerIdentifier,
		)
		Expect(found).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())

		Expect(actualContainer.Handle).To(Equal("some-handle"))
		Expect(actualContainer.WorkerName).To(Equal(containerToCreate.WorkerName))
		Expect(actualContainer.ResourceID).To(Equal(containerToCreate.ResourceID))

		By("returning a single matching step container info")
		actualStepContainer, found, err := database.FindContainerByIdentifier(
			stepContainerToCreate.ContainerIdentifier,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(actualStepContainer.Handle).To(Equal("other-handle"))
		Expect(actualStepContainer.WorkerName).To(Equal(stepContainerToCreate.WorkerName))
		Expect(actualStepContainer.ResourceID).To(Equal(stepContainerToCreate.ResourceID))

		By("erroring if more than one container matches the filter")
		matchingContainerToCreate := db.Container{
			ContainerIdentifier: db.ContainerIdentifier{
				Stage:       db.ContainerStageRun,
				CheckType:   "some-type",
				CheckSource: atc.Source{"some": "other-source"},
				BuildID:     1234,
				ResourceID:  getResourceID("some-resource"),
			},
			ContainerMetadata: db.ContainerMetadata{
				Handle:       "matching-handle",
				PipelineName: "some-pipeline",
				ResourceName: "some-resource",
				WorkerName:   "some-worker",
				Type:         db.ContainerTypeCheck,
			},
		}

		createdMatchingContainer, err := database.CreateContainer(matchingContainerToCreate, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		foundContainer, found, err := database.FindContainerByIdentifier(
			db.ContainerIdentifier{
				ResourceID: createdMatchingContainer.ResourceID,
			})
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(db.ErrMultipleContainersFound))
		Expect(found).To(BeFalse())
		Expect(foundContainer.Handle).To(BeEmpty())

		By("erroring if not enough identifiers are passed in")
		foundContainer, found, err = database.FindContainerByIdentifier(
			db.ContainerIdentifier{
				BuildID: createdMatchingContainer.BuildID,
			})
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(foundContainer.Handle).To(BeEmpty())

		By("returning found of false if no containers match the filter")
		actualContainer, found, err = database.FindContainerByIdentifier(
			db.ContainerIdentifier{
				BuildID: -1,
				PlanID:  atc.PlanID("plan-id"),
				Stage:   db.ContainerStageRun,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
		Expect(actualContainer.Handle).To(BeEmpty())

		By("removing it if the TTL has expired")
		ttl := 1 * time.Second

		err = database.UpdateExpiresAtOnContainer(otherHandle, -ttl)
		Expect(err).NotTo(HaveOccurred())
		_, found, err = database.FindContainerByIdentifier(
			stepContainerToCreate.ContainerIdentifier,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeFalse())
	})
})

func getAllContainers(sqldb db.Conn) []db.Container {
	var container_slice []db.Container
	query := `SELECT worker_name, pipeline_id, resource_id, build_id, plan_id
	          FROM containers
						`
	rows, err := sqldb.Query(query)
	Expect(err).NotTo(HaveOccurred())
	defer rows.Close()

	for rows.Next() {
		var container db.Container
		rows.Scan(&container.WorkerName, &container.ResourceID, &container.BuildID, &container.PlanID)
		container_slice = append(container_slice, container)
	}
	return container_slice
}

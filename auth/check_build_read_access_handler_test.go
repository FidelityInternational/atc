package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/concourse/atc"
	"github.com/concourse/atc/auth"
	"github.com/concourse/atc/auth/authfakes"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/dbfakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CheckBuildReadAccessHandler", func() {
	var (
		response       *http.Response
		server         *httptest.Server
		delegate       *buildDelegateHandler
		buildsDB       *authfakes.FakeBuildsDB
		handlerFactory auth.CheckBuildReadAccessHandlerFactory
		handler        http.Handler

		authValidator     *authfakes.FakeValidator
		userContextReader *authfakes.FakeUserContextReader

		build    *dbfakes.FakeBuild
		pipeline db.SavedPipeline
	)

	BeforeEach(func() {
		buildsDB = new(authfakes.FakeBuildsDB)
		handlerFactory = auth.NewCheckBuildReadAccessHandlerFactory(buildsDB)

		authValidator = new(authfakes.FakeValidator)
		userContextReader = new(authfakes.FakeUserContextReader)

		delegate = &buildDelegateHandler{}

		build = new(dbfakes.FakeBuild)
		pipeline = db.SavedPipeline{}
		build.GetPipelineReturns(pipeline, nil)
		build.TeamNameReturns("some-team")
		build.JobNameReturns("some-job")
	})

	JustBeforeEach(func() {
		server = httptest.NewServer(handler)

		request, err := http.NewRequest("POST", server.URL+"?:team_name=some-team&:build_id=55", nil)
		Expect(err).NotTo(HaveOccurred())

		response, err = new(http.Client).Do(request)
		Expect(err).NotTo(HaveOccurred())
	})

	var _ = AfterEach(func() {
		server.Close()
	})

	ItReturnsTheBuild := func() {
		It("returns 200 ok", func() {
			Expect(response.StatusCode).To(Equal(http.StatusOK))
		})

		It("calls delegate with the build context", func() {
			Expect(delegate.IsCalled).To(BeTrue())
			Expect(delegate.ContextBuild).To(BeIdenticalTo(build))
		})
	}

	WithExistingBuild := func(buildExistsFunc func()) {
		Context("when build exists", func() {
			BeforeEach(func() {
				buildsDB.GetBuildByIDReturns(build, true, nil)
			})

			buildExistsFunc()
		})

		Context("when build is not found", func() {
			BeforeEach(func() {
				buildsDB.GetBuildByIDReturns(nil, false, nil)
			})

			It("returns 404", func() {
				Expect(response.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Context("when getting build fails", func() {
			BeforeEach(func() {
				buildsDB.GetBuildByIDReturns(nil, false, errors.New("disaster"))
			})

			It("returns 404", func() {
				Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
			})
		})
	}

	Context("AnyJobHandler", func() {
		BeforeEach(func() {
			checkBuildReadAccessHandler := handlerFactory.AnyJobHandler(delegate, auth.UnauthorizedRejector{})
			handler = auth.WrapHandler(checkBuildReadAccessHandler, authValidator, userContextReader)
		})

		Context("when authenticated and accessing same team's build", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(true)
				userContextReader.GetTeamReturns("some-team", 42, true, true)
			})

			WithExistingBuild(ItReturnsTheBuild)
		})

		Context("when authenticated but accessing different team's build", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(true)
				userContextReader.GetTeamReturns("other-team-name", 0, false, true)
			})

			WithExistingBuild(func() {
				Context("when pipeline is public", func() {
					BeforeEach(func() {
						build.GetPipelineReturns(db.SavedPipeline{Public: true}, nil)
					})

					ItReturnsTheBuild()
				})

				Context("when pipeline is private", func() {
					BeforeEach(func() {
						build.GetPipelineReturns(db.SavedPipeline{Public: false}, nil)
					})

					It("returns 403", func() {
						Expect(response.StatusCode).To(Equal(http.StatusForbidden))
					})
				})
			})
		})

		Context("when not authenticated", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(false)
				userContextReader.GetTeamReturns("", 0, false, false)
			})

			WithExistingBuild(func() {
				Context("when pipeline is public", func() {
					BeforeEach(func() {
						build.GetPipelineReturns(db.SavedPipeline{Public: true}, nil)
					})

					ItReturnsTheBuild()
				})

				Context("when pipeline is private", func() {
					BeforeEach(func() {
						build.GetPipelineReturns(db.SavedPipeline{Public: false}, nil)
					})

					It("returns 401", func() {
						Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
					})
				})
			})
		})
	})

	Context("CheckIfPrivateJobHandler", func() {
		BeforeEach(func() {
			checkBuildReadAccessHandler := handlerFactory.CheckIfPrivateJobHandler(delegate, auth.UnauthorizedRejector{})
			handler = auth.WrapHandler(checkBuildReadAccessHandler, authValidator, userContextReader)
		})

		ItChecksIfJobIsPrivate := func(status int) {
			Context("when pipeline is public", func() {
				BeforeEach(func() {
					build.GetPipelineReturns(db.SavedPipeline{Public: true}, nil)
				})

				Context("when getting build config fails", func() {
					BeforeEach(func() {
						build.GetConfigReturns(atc.Config{}, 0, errors.New("disaster"))
					})

					It("returns 500", func() {
						Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
					})
				})

				Context("getting build config succeeds", func() {
					var config atc.Config

					Context("and job is public", func() {
						BeforeEach(func() {
							config = atc.Config{
								Jobs: []atc.JobConfig{
									{
										Name:   "some-job",
										Public: true,
									},
								},
							}
							build.GetConfigReturns(config, 1, nil)
						})

						ItReturnsTheBuild()
					})

					Context("and job is private", func() {
						BeforeEach(func() {
							config = atc.Config{
								Jobs: []atc.JobConfig{
									{
										Name:   "some-job",
										Public: false,
									},
								},
							}
							build.GetConfigReturns(config, 1, nil)
						})

						It("returns "+string(status), func() {
							Expect(response.StatusCode).To(Equal(status))
						})
					})

					Context("checking job is public fails", func() {
						BeforeEach(func() {
							build.GetConfigReturns(atc.Config{}, 0, errors.New("disaster"))
						})

						It("returns 500", func() {
							Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
						})
					})
				})
			})

			Context("when pipeline is private", func() {
				BeforeEach(func() {
					build.GetPipelineReturns(db.SavedPipeline{Public: false}, nil)
				})

				It("returns "+string(status), func() {
					Expect(response.StatusCode).To(Equal(status))
				})
			})
		}

		Context("when authenticated and accessing same team's build", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(true)
				userContextReader.GetTeamReturns("some-team", 42, true, true)
			})

			WithExistingBuild(ItReturnsTheBuild)
		})

		Context("when authenticated but accessing different team's build", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(true)
				userContextReader.GetTeamReturns("other-team-name", 0, false, true)
			})

			WithExistingBuild(func() {
				ItChecksIfJobIsPrivate(http.StatusForbidden)
			})
		})

		Context("when not authenticated", func() {
			BeforeEach(func() {
				authValidator.IsAuthenticatedReturns(false)
				userContextReader.GetTeamReturns("", 0, false, false)
			})

			WithExistingBuild(func() {
				ItChecksIfJobIsPrivate(http.StatusUnauthorized)
			})
		})
	})
})

type buildDelegateHandler struct {
	IsCalled     bool
	ContextBuild db.Build
}

func (handler *buildDelegateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler.IsCalled = true
	handler.ContextBuild = r.Context().Value(auth.BuildKey).(db.Build)
}

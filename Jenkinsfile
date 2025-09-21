pipeline {
    agent {
        kubernetes {
            yaml '''
                apiVersion: v1
                kind: Pod
                spec:
                  containers:
                  - name: zaraos-builder
                    image: koalby/zaraos-builder:nightly # FIXME: change this once we are in prod
                    imagePullPolicy: Always # FIXME: change this once we are in prod
                    command:
                    - sleep
                    args:
                    - 99d
                    tty: true
                    securityContext:
                      runAsUser: 0
                  - name: jnlp
                    image: jenkins/inbound-agent:latest
            '''
        }
    }

    triggers {
        cron('0 3 * * *')
    }

    environment {
        BUILD_DIR = '/tmp/zaraos-build'
        DL_DIR = '/tmp/zaraos-dl'
        IS_NIGHTLY = "${env.BUILD_CAUSE == 'TIMERTRIGGER'}"
        IS_MAIN_BRANCH = "${env.BRANCH_NAME == 'main'}"
        IS_DEVELOP_BRANCH = "${env.BRANCH_NAME == 'develop'}"
        IS_PR = "${env.CHANGE_ID != null}"
    }

    options {
        buildDiscarder(logRotator(numToKeepStr: '10'))
        timeout(time: 120, unit: 'MINUTES')
        skipDefaultCheckout()
    }

    stages {
        stage('Setup') {
            steps {
                script {
                    def checkName = env.IS_NIGHTLY == 'true' ? 'ZaraOS Nightly Build' :
                                   env.IS_PR == 'true' ? 'ZaraOS PR Check' : 'ZaraOS Release Build'

                    publishChecks name: checkName, status: 'IN_PROGRESS',
                                summary: 'Building ZaraOS for Raspberry Pi 5...'
                }

                checkout scm

                container('zaraos-builder') {
                    sh '''
                        echo "ZaraOS Build Environment Ready"
                        whoami
                        pwd
                        ls -la /opt/buildroot/
                        rm -rf ${BUILD_DIR} ${DL_DIR} output || true
                        mkdir -p ${BUILD_DIR} ${DL_DIR} output
                    '''
                }
            }
        }

        // stage('Configure Build') {
        //     steps {
        //         container('zaraos-builder') {
        //             sh '''
        //                 make -C /opt/buildroot \\
        //                     O="${BUILD_DIR}" \\
        //                     BR2_DL_DIR="${DL_DIR}" \\
        //                     BR2_EXTERNAL="$(pwd)/ZaraOS" \\
        //                     zaraos_pi5_defconfig
        //             '''
        //         }
        //     }
        // }

        // stage('Build ZaraOS') {
        //     steps {
        //         container('zaraos-builder') {
        //             sh '''
        //                 CPU_COUNT=$(nproc)
        //                 JOBS=$(( CPU_COUNT < 8 ? CPU_COUNT : 8 ))
        //                 echo "Using $JOBS parallel jobs"

        //                 make -C /opt/buildroot \\
        //                     O="${BUILD_DIR}" \\
        //                     BR2_DL_DIR="${DL_DIR}" \\
        //                     BR2_EXTERNAL="$(pwd)/ZaraOS" \\
        //                     -j${JOBS}
        //             '''
        //         }
        //     }
        // }

        // stage('Package Artifacts') {
        //     steps {
        //         container('zaraos-builder') {
        //             sh '''
        //                 if [[ ! -f "${BUILD_DIR}/images/sdcard.img" ]]; then
        //                     echo "ERROR: sdcard.img not found!"
        //                     exit 1
        //                 fi

        //                 cp -r "${BUILD_DIR}/images"/* output/
        //                 ls -la output/
        //                 du -h output/*
        //             '''

        //             archiveArtifacts artifacts: "output/*", fingerprint: true
        //         }
        //     }
        // }

        stage('Create GitHub Release') {
            when {
                anyOf {
                    allOf {
                        environment name: 'IS_MAIN_BRANCH', value: 'true'
                        not { environment name: 'IS_PR', value: 'true' }
                    }
                    environment name: 'IS_NIGHTLY', value: 'true'
                    environment name: 'IS_DEVELOP_BRANCH', value: 'true'
                }
            }
            steps {
                container('zaraos-builder') {
                    script {
                        // Get commit hash from git command
                        sh 'git config --global --add safe.directory "*"'
                        def commitHash = sh(script: 'git rev-parse HEAD', returnStdout: true).trim()
                        
                        def releaseTag
                        def releaseName
                        def isPrerelease
                        def shortSha = commitHash.take(8)
        
                        // Rest of your existing logic...
                        if (env.IS_NIGHTLY == 'true') {
                            releaseTag = "nightly-${new Date().format('yyyyMMdd-HHmmss')}"
                            releaseName = "ZaraOS Nightly ${releaseTag}"
                            isPrerelease = true
                        } else if (env.IS_DEVELOP_BRANCH == 'true') {
                            def timestamp = new Date().format('yyyyMMdd-HHmmss')
                            releaseTag = "develop-${timestamp}-${shortSha}"
                            releaseName = "ZaraOS Development ${releaseTag}"
                            isPrerelease = true
                        } else {
                            def timestamp = new Date().format('yyyyMMdd-HHmmss')
                            releaseTag = "v${timestamp}-${shortSha}"
                            releaseName = "ZaraOS ${releaseTag}"
                            isPrerelease = false
                        }
        
                        def releaseBody = """
        ## ZaraOS Release ${releaseTag}
        
        Automated build from commit ${commitHash}
        
        ### Build Information
        - **Branch**: ${env.BRANCH_NAME}
        - **Commit**: ${commitHash}
        - **Build Date**: ${new Date().format('yyyy-MM-dd HH:mm:ss')}
        - **Build Number**: ${env.BUILD_NUMBER}
        - **Target**: Raspberry Pi 5 (64-bit ARM)
        
        ### Installation
        Flash the sdcard.img to an SD card using Raspberry Pi Imager or dd command.
                        """.trim()
        
                        writeFile file: 'release_body.md', text: releaseBody
        
                        createGitHubRelease(
                            credentialId: 'github_personal_token',
                            repository: 'KoalbyMQP/ZaraOS',
                            tag: releaseTag,
                            name: releaseName,
                            bodyFile: 'release_body.md',
                            prerelease: isPrerelease,
                            draft: false,
                            commitish: commitHash  // Use the retrieved commit hash
                        )
        
                        // Upload assets
                        uploadGithubReleaseAsset(
                            credentialId: 'github_personal_token',
                            repository: 'KoalbyMQP/ZaraOS',
                            tagName: releaseTag,
                            commitish: commitHash,  // Use the retrieved commit hash
                            uploadAssets: [
                                [filePath: 'output/sdcard.img']
                            ]
                        )
        
                        env.RELEASE_TAG = releaseTag
                        echo "Release created: ${releaseName}"
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                def checkName = env.IS_NIGHTLY == 'true' ? 'ZaraOS Nightly Build' :
                               env.IS_PR == 'true' ? 'ZaraOS PR Check' : 'ZaraOS Release Build'

                def conclusion = currentBuild.currentResult == 'SUCCESS' ? 'SUCCESS' : 'FAILURE'
                def summary = currentBuild.currentResult == 'SUCCESS' ?
                            'ZaraOS build completed successfully' : 'ZaraOS build failed - check logs for details'

                publishChecks name: checkName, conclusion: conclusion, summary: summary
            }
        }

        success {
            script {
                if (env.RELEASE_TAG) {
                    echo "ZaraOS ${env.RELEASE_TAG} built and released successfully!"
                    echo "Download: https://github.com/${env.CHANGE_TARGET ?: env.GIT_URL.tokenize('/').last().replace('.git', '')}/releases/tag/${env.RELEASE_TAG}"
                } else {
                    echo "ZaraOS build completed successfully!"
                }
            }
        }

        failure {
            echo "ZaraOS build failed. Check the build logs for details."
        }
    }
}

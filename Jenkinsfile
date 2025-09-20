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
                        def releaseTag
                        def releaseName
                        def isPrerelease

                        if (env.IS_NIGHTLY == 'true') {
                            releaseTag = "nightly-${new Date().format('yyyyMMdd-HHmmss')}"
                            releaseName = "ZaraOS Nightly ${releaseTag}"
                            isPrerelease = true
                        } else if (env.IS_DEVELOP_BRANCH == 'true') {
                            def timestamp = new Date().format('yyyyMMdd-HHmmss')
                            def shortSha = env.GIT_COMMIT.take(8)
                            releaseTag = "develop-${timestamp}-${shortSha}"
                            releaseName = "ZaraOS Development ${releaseTag}"
                            isPrerelease = true
                        } else {
                            def timestamp = new Date().format('yyyyMMdd-HHmmss')
                            def shortSha = env.GIT_COMMIT.take(8)
                            releaseTag = "v${timestamp}-${shortSha}"
                            releaseName = "ZaraOS ${releaseTag}"
                            isPrerelease = false
                        }

                        env.RELEASE_TAG = releaseTag
                        env.RELEASE_NAME = releaseName
                        env.IS_PRERELEASE = isPrerelease.toString()

                        withCredentials([string(credentialsId: 'Jenkins_Github', variable: 'GITHUB_TOKEN')]) {
                            sh '''
                                echo "Creating GitHub release: ${RELEASE_NAME}"

                                echo "${GITHUB_TOKEN}" | gh auth login --with-token

                                # Check if release already exists
                                if gh release view "${RELEASE_TAG}" >/dev/null 2>&1; then
                                    echo "Release ${RELEASE_TAG} already exists, skipping"
                                    exit 0
                                fi

                                cat > release_body.md << 'EOF'
## ZaraOS Release ${RELEASE_TAG}

Automated build from commit ${GIT_COMMIT}

### Build Information
- **Branch**: ${BRANCH_NAME}
- **Commit**: ${GIT_COMMIT}
- **Build Date**: $(date -u)
- **Build Number**: ${BUILD_NUMBER}
- **Target**: Raspberry Pi 5 (64-bit ARM)

### Included Files
- `sdcard.img` - Complete SD card image for Raspberry Pi 5
- `Image` - Linux kernel image (64-bit ARM)
- `rootfs.ext4` - Root filesystem
- `*.dtb` - Device tree blobs for Raspberry Pi models
- `boot.vfat` - Boot partition image

### Installation
Flash the `sdcard.img` to an SD card using tools like Raspberry Pi Imager or `dd`:
```bash
sudo dd if=sdcard.img of=/dev/sdX bs=4M status=progress
```
Replace `/dev/sdX` with your SD card device.

### Changes
- Based on Buildroot with ZaraOS customizations
- Python 3 runtime included
- Optimized for robotics applications
- Auto-login enabled for development

EOF

                                PRERELEASE_FLAG=""
                                if [ "${IS_PRERELEASE}" = "true" ]; then
                                    PRERELEASE_FLAG="--prerelease"
                                fi

                                gh release create "${RELEASE_TAG}" \\
                                    --title "${RELEASE_NAME}" \\
                                    --notes-file release_body.md \\
                                    ${PRERELEASE_FLAG} \\
                                    output/sdcard.img \\
                                    output/Image \\
                                    output/rootfs.ext4 \\
                                    output/*.dtb \\
                                    output/boot.vfat

                                echo "Release created successfully: ${RELEASE_TAG}"
                            '''
                        }
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

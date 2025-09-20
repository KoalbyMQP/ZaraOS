pipeline {
    agent {
        kubernetes {
            yaml '''
                apiVersion: v1
                kind: Pod
                spec:
                  containers:
                  - name: zaraos-builder
                    image: koalby/zaraos-builder:latest
                    command:
                    - sleep
                    args:
                    - 99d
                    tty: true
                  - name: jnlp
                    image: jenkins/inbound-agent:latest
            '''
        }
    }

    triggers {
        cron('0 3 * * *')  // Nightly build at 3 AM
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
                        echo "Workspace: $(pwd)"
                        echo "Available space: $(df -h . | tail -1)"

                        # Verify buildroot is available
                        ls -la /opt/buildroot/

                        # Clean any previous builds
                        rm -rf ${BUILD_DIR} ${DL_DIR} output || true
                        mkdir -p ${BUILD_DIR} ${DL_DIR} output
                    '''
                }
            }
        }

        stage('Prepare Nightly Version') {
            when {
                environment name: 'IS_NIGHTLY', value: 'true'
            }
            steps {
                container('zaraos-builder') {
                    script {
                        def timestamp = new Date().format('yyyyMMddHHmm')
                        env.ZARAOS_VERSION = "nightly-${timestamp}"

                        sh """
                            echo "Building nightly version: ${env.ZARAOS_VERSION}"

                            # Update version in zaraos-release file
                            sed -i 's/Build System: Container-based CI\\/CD/Build System: Nightly ${env.ZARAOS_VERSION}/' ZaraOS/overlays/etc/zaraos-release
                        """
                    }
                }
            }
        }

        stage('Configure Build') {
            steps {
                container('zaraos-builder') {
                    sh '''
                        echo "Configuring ZaraOS build"

                        # Configure buildroot with ZaraOS external tree
                        make -C /opt/buildroot \\
                            O="${BUILD_DIR}" \\
                            BR2_DL_DIR="${DL_DIR}" \\
                            BR2_EXTERNAL="$(pwd)/ZaraOS" \\
                            zaraos_pi5_defconfig

                        echo "Configuration complete"
                    '''
                }
            }
        }

        stage('Build ZaraOS') {
            steps {
                container('zaraos-builder') {
                    sh '''
                        echo "Building ZaraOS for Raspberry Pi 5"
                        echo "This may take 30-60 minutes..."

                        # Get CPU count for parallel build
                        CPU_COUNT=$(nproc)
                        JOBS=$(echo "$CPU_COUNT * 0.75" | bc | cut -d. -f1)
                        JOBS=$(( JOBS < 2 ? 2 : JOBS ))
                        JOBS=$(( JOBS > 8 ? 8 : JOBS ))
                        echo "Using $JOBS parallel jobs (CPU count: $CPU_COUNT)"

                        # Build ZaraOS
                        make -C /opt/buildroot \\
                            O="${BUILD_DIR}" \\
                            BR2_DL_DIR="${DL_DIR}" \\
                            BR2_EXTERNAL="$(pwd)/ZaraOS" \\
                            -j${JOBS} \\
                            V=1

                        echo "Build completed successfully"
                    '''
                }
            }
        }

        stage('Package Artifacts') {
            steps {
                container('zaraos-builder') {
                    sh '''
                        echo "Packaging ZaraOS artifacts"

                        # Verify build artifacts exist
                        if [[ ! -f "${BUILD_DIR}/images/sdcard.img" ]]; then
                            echo "ERROR: sdcard.img not found!"
                            exit 1
                        fi

                        # Copy all artifacts to output directory
                        echo "Copying build artifacts..."
                        cp -r "${BUILD_DIR}/images"/* output/

                        # List what we built
                        echo "Built artifacts:"
                        ls -la output/

                        # Calculate file sizes
                        echo "Artifact sizes:"
                        du -h output/*
                    '''

                    archiveArtifacts artifacts: "output/*", fingerprint: true
                }
            }
        }

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
                            releaseTag = env.ZARAOS_VERSION ?: "nightly-${new Date().format('yyyyMMdd-HHmmss')}"
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

                                # Install GitHub CLI if not available
                                if ! command -v gh &> /dev/null; then
                                    echo "Installing GitHub CLI..."
                                    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg
                                    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
                                    apt update && apt install -y gh
                                fi

                                # Set up GitHub authentication using App credentials
                                echo "${GITHUB_TOKEN}" | gh auth login --with-token

                                # Create release body
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

                                # Create the release
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

pipeline {
    agent any

    environment {
        DOCKER_ORG   = 'lifegoeson34'
        SERVICE_NAME = 'be-modami-chat-service'
        FULL_IMAGE   = "${DOCKER_ORG}/${SERVICE_NAME}"
    }

    options {
        buildDiscarder(logRotator(numToKeepStr: '10'))
        timeout(time: 30, unit: 'MINUTES')
        disableConcurrentBuilds()
    }

    triggers {
        githubPush()
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
                script {
                    env.IMAGE_TAG = "git-${sh(script: 'git rev-parse --short=7 HEAD', returnStdout: true).trim()}"
                }
                echo "Building image: ${FULL_IMAGE}:${env.IMAGE_TAG}"
            }
        }

        stage('Secret Scan') {
            steps {
                sh '''
                    docker run --rm \
                        -v "$(pwd):/path" \
                        zricethezav/gitleaks:latest detect \
                        --source /path \
                        --no-git \
                        --exit-code 1
                '''
            }
        }

        stage('Build') {
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'dockerhub-credentials',
                        usernameVariable: 'DOCKER_USER',
                        passwordVariable: 'DOCKER_PASS'
                    ),
                    usernamePassword(
                        credentialsId: 'gitlab-credentials',
                        usernameVariable: 'GITLAB_USER',
                        passwordVariable: 'GITLAB_TOKEN'
                    )
                ]) {
                    sh '''
                        echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin

                        DOCKER_BUILDKIT=1 docker build \
                            --secret id=gitlab_username,env=GITLAB_USER \
                            --secret id=gitlab_token,env=GITLAB_TOKEN \
                            --platform linux/amd64 \
                            --build-arg BUILDPLATFORM=linux/amd64 \
                            --build-arg TARGETOS=linux \
                            --build-arg TARGETARCH=amd64 \
                            -t "${FULL_IMAGE}:${IMAGE_TAG}" \
                            -t "${FULL_IMAGE}:latest" \
                            .
                    '''
                }
            }
        }

        stage('Image Scan') {
            steps {
                sh '''
                    docker run --rm \
                        -v /var/run/docker.sock:/var/run/docker.sock \
                        aquasec/trivy:latest image \
                        --exit-code 1 \
                        --severity HIGH,CRITICAL \
                        --no-progress \
                        "${FULL_IMAGE}:${IMAGE_TAG}"
                '''
            }
        }

        stage('Push') {
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'dockerhub-credentials',
                        usernameVariable: 'DOCKER_USER',
                        passwordVariable: 'DOCKER_PASS'
                    )
                ]) {
                    sh '''
                        echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin
                        docker push "${FULL_IMAGE}:${IMAGE_TAG}"
                        docker push "${FULL_IMAGE}:latest"
                        docker logout
                    '''
                }
            }
        }
    }

    post {
        always {
            sh 'docker image prune -f || true'
        }
        success {
            echo "Successfully pushed ${FULL_IMAGE}:${env.IMAGE_TAG}"
        }
        failure {
            echo "Pipeline failed for ${SERVICE_NAME}"
        }
    }
}

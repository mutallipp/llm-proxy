@echo off
setlocal

set RELEASE_NAME=%1
if "%RELEASE_NAME%"=="" set RELEASE_NAME=llm-proxy

set NAMESPACE=%2
if "%NAMESPACE%"=="" set NAMESPACE=default

echo Installing llm-proxy with Helm...
echo Release name: %RELEASE_NAME%
echo Namespace: %NAMESPACE%

REM Create namespace if it doesn't exist
kubectl create namespace %NAMESPACE% 2>nul || echo Namespace already exists

REM Install the chart
helm install %RELEASE_NAME% ./deploy/helm ^
  --namespace %NAMESPACE% ^
  --timeout 10m0s

echo.
echo Installation completed!
echo.
echo To access llm-proxy:
echo 1. Port forward the service:
echo    kubectl port-forward svc/%RELEASE_NAME% 8090:8090 -n %NAMESPACE%
echo.
echo 2. Visit http://localhost:8090 in your browser
echo.
echo To check the status:
echo    kubectl get pods -n %NAMESPACE%
echo.
echo To view logs:
echo    kubectl logs -l app.kubernetes.io/name=llm-proxy -n %NAMESPACE%
echo.
echo To uninstall:
echo    helm uninstall %RELEASE_NAME% -n %NAMESPACE%
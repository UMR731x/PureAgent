@echo off
cd /d "%~dp0"
bin\PureAgent.exe %*
if errorlevel 1 pause

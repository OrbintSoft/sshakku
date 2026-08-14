@echo off
rem A stand-in for the user's editor, for the `sshakku config --edit` tests:
rem this system's counterpart to editor.sh, doing the same thing so the tests
rem that drive it are one set rather than two. A real program, run by SSHakku
rem the way any editor is, that records what it was asked to open and then
rem saves a prepared file over it.
rem
rem It is driven by the environment rather than by arguments so that a test can
rem pass arguments of its own through %EDITOR% and see them arrive here:
rem
rem   SSHAKKU_TEST_EDITOR_RECORD   file to append this invocation's arguments to
rem   SSHAKKU_TEST_EDITOR_BODY     file to save over the last argument; unset or
rem                                empty leaves the file exactly as it was found
setlocal

rem The redirection comes first so that arguments ending in a digit are not read
rem as a stream number.
if not "%SSHAKKU_TEST_EDITOR_RECORD%"=="" >>"%SSHAKKU_TEST_EDITOR_RECORD%" echo %*

rem The file to save over is the last argument, whatever came before it.
set "target="
:lastarg
if "%~1"=="" goto saved
set "target=%~1"
shift
goto lastarg

:saved
if not "%SSHAKKU_TEST_EDITOR_BODY%"=="" copy /y "%SSHAKKU_TEST_EDITOR_BODY%" "%target%" >nul

endlocal
exit /b 0

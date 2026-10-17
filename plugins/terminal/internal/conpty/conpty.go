// Package conpty finds the ConPTY that Windows apps ship, conpty.dll and
// its console host OpenConsole.exe, of the Windows Terminal team's
// package on NuGet (Microsoft.Windows.Console.ConPTY). Windows 10's own
// ConPTY renders the screen again rather than pass the program's output
// through: it drops the modes of full-screen programs (the mouse, the
// alternate screen), splits their synchronized updates and skips frames.
//
// Its mygo-plugin.json names the builds, which `mygo build` puts among the
// resources of Windows apps: only package pty imports this one, and only
// on Windows, so that apps for other platforms don't ship them.
package conpty

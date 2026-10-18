ANIIMO WEATHER FOR WINDOWS
==========================

1. Extract the complete AniimoWeather folder from the zip.
2. Double-click AniimoWeather.exe.
3. Enter your Discord webhook and player name in Settings.
4. Click "Save and restart monitor".
5. Open Aniimo's weather forecast and take a Steam screenshot (F12 by default).

The activity panel will say whether an Aniimo screenshot folder was found. If
it was not, expand Advanced and add the Steam userdata folder shown by Steam's
installation (for example D:\Steam\userdata), then save and restart the monitor.
For future bug reports, Advanced also has an optional Diagnostic log file field.
Choose a writable path, reproduce the problem, then attach that log file.

Keep the tesseract folder beside AniimoWeather.exe. It contains the bundled OCR
engine, so no separate Go, Python, or Tesseract installation is required.

Closing the window hides the application in the system tray. Use Quit from the
tray menu when you want to stop monitoring.

WINDOWS SECURITY NOTICE
-----------------------

This community-built executable is not code-signed. Microsoft Defender
SmartScreen may therefore show "Unknown publisher" or "Windows protected your
PC", particularly while a release has little download reputation. That warning
alone does not mean malware was detected.

Download only from the official GitHub Releases page and verify the ZIP using
the accompanying .sha256 file. Do not bypass a warning that gives a specific
malware or threat name; report that detection and the release version so it can
be investigated. Avoid copies redistributed by third parties.

This package includes Tesseract OCR and its runtime dependencies. Their license
and notice files are retained inside the tesseract folder.

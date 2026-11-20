# Windows PowerShell 5.1 built-in Forms, run in a single-threaded apartment.
$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$request = [Console]::In.ReadToEnd() | ConvertFrom-Json
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Windows.Forms.Application]::EnableVisualStyles()

$title = 'Tune Fishin · Check my gig USB'
$result = @{ canceled = $false; value = '' }
if ($request.kind -eq 'file' -or $request.kind -eq 'folder') {
    if ($request.kind -eq 'file') {
        $dialog = [Windows.Forms.OpenFileDialog]::new()
        $dialog.Title = $request.prompt
        $dialog.Filter = 'Playlists (*.m3u8, *.m3u)|*.m3u8;*.m3u|All files (*.*)|*.*'
    } else {
        $dialog = [Windows.Forms.FolderBrowserDialog]::new()
        $dialog.Description = $request.prompt
        $dialog.ShowNewFolderButton = $false
    }
    try {
        if ($dialog.ShowDialog() -eq [Windows.Forms.DialogResult]::OK) {
            if ($request.kind -eq 'file') { $result.value = $dialog.FileName }
            else { $result.value = $dialog.SelectedPath }
        } else { $result.canceled = $true }
    } finally { $dialog.Dispose() }
} else {
    $form = [Windows.Forms.Form]::new()
    try {
        $form.Text = $title
        $form.StartPosition = 'CenterScreen'
        $form.Font = [Drawing.SystemFonts]::MessageBoxFont
        $form.AutoScaleMode = 'Font'
        $form.MinimumSize = [Drawing.Size]::new(460, 180)
        $form.ClientSize = [Drawing.Size]::new(560, 170)
        $form.Padding = [Windows.Forms.Padding]::new(12)
        $form.MaximizeBox = $false

        $buttons = [Windows.Forms.FlowLayoutPanel]::new()
        $buttons.Dock = 'Bottom'
        $buttons.AutoSize = $true
        $buttons.FlowDirection = 'RightToLeft'
        $ok = [Windows.Forms.Button]::new()
        $ok.Text = 'Continue'
        $ok.AutoSize = $true
        $ok.DialogResult = 'OK'
        $buttons.Controls.Add($ok)
        $cancel = [Windows.Forms.Button]::new()
        $cancel.Text = 'Cancel'
        $cancel.AutoSize = $true
        $cancel.DialogResult = 'Cancel'
        $buttons.Controls.Add($cancel)
        $form.AcceptButton = $ok
        $form.CancelButton = $cancel

        $label = [Windows.Forms.Label]::new()
        $label.Text = $request.prompt
        $label.Dock = 'Top'
        $label.AutoSize = $true
        $label.MaximumSize = [Drawing.Size]::new(520, 0)
        $label.Padding = [Windows.Forms.Padding]::new(0, 0, 0, 8)
        $editor = $null
        switch ($request.kind) {
            'choice' {
                $editor = [Windows.Forms.ComboBox]::new()
                $editor.DropDownStyle = 'DropDownList'
                $editor.Items.AddRange([object[]]$request.choices)
                if ($request.default) { $editor.SelectedItem = $request.default }
                $ok.Enabled = $editor.SelectedIndex -ge 0
                $editor.Add_SelectedIndexChanged({ $ok.Enabled = $editor.SelectedIndex -ge 0 })
                $editor.Dock = 'Top'
            }
            'entry' {
                $editor = [Windows.Forms.TextBox]::new()
                $editor.Text = $request.default
                $editor.Dock = 'Top'
            }
            'report' {
                $form.ClientSize = [Drawing.Size]::new(740, 460)
                $form.MaximizeBox = $true
                $editor = [Windows.Forms.TextBox]::new()
                $editor.Multiline = $true
                $editor.ReadOnly = $true
                $editor.ScrollBars = 'Vertical'
                $editor.Dock = 'Fill'
                $editor.Text = $request.default -replace '\r?\n', "`r`n"
                $ok.Text = 'Done'
                $cancel.Visible = $false
            }
            'busy' {
                $ok.Visible = $false
                $cancel.Text = 'Cancel check'
                $form.AcceptButton = $cancel
            }
            default { throw 'Unknown dialog type' }
        }
        if ($editor) { $form.Controls.Add($editor) }
        $form.Controls.Add($label)
        $form.Controls.Add($buttons)
        $form.Add_Shown({ $form.Activate() })
        if ($form.ShowDialog() -eq [Windows.Forms.DialogResult]::OK) {
            if ($request.kind -eq 'choice') { $result.value = [string]$editor.SelectedItem }
            if ($request.kind -eq 'entry') { $result.value = $editor.Text }
        } else { $result.canceled = $true }
    } finally { $form.Dispose() }
}
[Console]::Write(($result | ConvertTo-Json -Compress))

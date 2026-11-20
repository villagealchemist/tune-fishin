// macOS JavaScript for Automation, run by /usr/bin/osascript.
ObjC.import('AppKit');

function run() {
    const input = $.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile;
    const request = JSON.parse(ObjC.unwrap($.NSString.alloc.initWithDataEncoding(input, $.NSUTF8StringEncoding)));
    const app = Application.currentApplication();
    app.includeStandardAdditions = true;
    const title = 'Tune Fishin · Check my gig USB';
    const nativeApp = $.NSApplication.sharedApplication;
    nativeApp.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
    nativeApp.activateIgnoringOtherApps(true);
    let value = '';
    try {
        switch (request.kind) {
            case 'choice': {
                const options = {withTitle: title, withPrompt: request.prompt, multipleSelectionsAllowed: false};
                if (request.default) options.defaultItems = [request.default];
                const selection = app.chooseFromList(request.choices, options);
                if (selection === false) return JSON.stringify({canceled: true});
                value = selection[0];
                break;
            }
            case 'file':
                value = app.chooseFile({withPrompt: request.prompt}).toString();
                break;
            case 'folder':
                value = app.chooseFolder({withPrompt: request.prompt}).toString();
                break;
            case 'entry':
                value = app.displayDialog(request.prompt, {
                    withTitle: title, defaultAnswer: request.default,
                    buttons: ['Cancel', 'Continue'], cancelButton: 'Cancel', defaultButton: 'Continue'
                }).textReturned;
                break;
            case 'busy':
                app.displayDialog(request.prompt, {withTitle: title, buttons: ['Cancel check'], cancelButton: 'Cancel check'});
                return JSON.stringify({canceled: true});
            case 'report': {
                const alert = $.NSAlert.alloc.init;
                alert.messageText = title;
                alert.informativeText = request.prompt;
                const scroll = $.NSScrollView.alloc.initWithFrame($.NSMakeRect(0, 0, 550, 340));
                scroll.hasVerticalScroller = true;
                scroll.borderType = $.NSBezelBorder;
                const area = $.NSTextView.alloc.initWithFrame($.NSMakeRect(0, 0, 530, 340));
                area.editable = false;
                area.selectable = true;
                area.verticallyResizable = true;
                area.autoresizingMask = $.NSViewWidthSizable;
                area.textContainer.widthTracksTextView = true;
                area.string = request.default;
                scroll.documentView = area;
                alert.accessoryView = scroll;
                alert.addButtonWithTitle('Done');
                alert.runModal;
                break;
            }
            default:
                throw new Error('Unknown dialog type');
        }
    } catch (err) {
        if (err.errorNumber === -128) return JSON.stringify({canceled: true});
        throw err;
    }
    return JSON.stringify({value: value});
}

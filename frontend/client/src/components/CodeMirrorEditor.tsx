import React, { useRef, useCallback, useEffect } from 'react';
import CodeMirror, { ReactCodeMirrorRef } from '@uiw/react-codemirror';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { EditorView } from '@codemirror/view';
import { useDocStore } from '../store/docStore';
import wsService from '../services/websocket';
import './CodeMirrorEditor.css';

interface CodeMirrorEditorProps {
  onDocChange?: (newContent: string) => void;
  readOnly?: boolean;
}

export const CodeMirrorEditor: React.FC<CodeMirrorEditorProps> = ({
  onDocChange,
  readOnly = false,
}) => {
  const content = useDocStore((s) => s.content);
  const role = useDocStore((s) => s.role);
  const remoteCursors = useDocStore((s) => s.remoteCursors);
  const setContent = useDocStore((s) => s.setContent);

  const cmRef = useRef<ReactCodeMirrorRef>(null);
  const lastContentRef = useRef<string>(content);
  const isRemoteApplyingRef = useRef<boolean>(false);

  const isViewer = readOnly || role === 'viewer';

  // Synchronize internal ref whenever content changes externally
  useEffect(() => {
    lastContentRef.current = content;
  }, [content]);

  // Handle local user edits inside CodeMirror
  const handleChange = useCallback(
    (value: string) => {
      if (isRemoteApplyingRef.current) return;
      if (isViewer) return;

      const prev = lastContentRef.current;
      if (value === prev) return;

      lastContentRef.current = value;
      setContent(value);

      if (onDocChange) {
        onDocChange(value);
      }
    },
    [isViewer, setContent, onDocChange]
  );

  // Broadcast cursor and selection positions
  const handleUpdate = useCallback((viewUpdate: any) => {
    if (viewUpdate.selectionSet && viewUpdate.state) {
      const mainSel = viewUpdate.state.selection.main;
      const headPos = mainSel.head;
      const lineObj = viewUpdate.state.doc.lineAt(headPos);
      const lineNumber = lineObj.number;
      const chNumber = headPos - lineObj.from;

      wsService.sendCursor({
        pos: headPos,
        sel_start: mainSel.from,
        sel_end: mainSel.to,
        line: lineNumber,
        ch: chNumber,
      });
    }
  }, []);

  return (
    <div className={`codemirror-wrapper ${isViewer ? 'codemirror-readonly' : ''}`}>
      {/* Remote collaborators cursor banner / badges */}
      {Object.keys(remoteCursors).length > 0 && (
        <div className="active-cursors-bar">
          <span className="cursors-label">Collaborating:</span>
          {Object.entries(remoteCursors).map(([clientId, info]) => (
            <span
              key={clientId}
              className="collaborator-cursor-tag"
              style={{ backgroundColor: info.color }}
            >
              {info.name} (Ln {info.cursor.line}, Col {info.cursor.ch})
            </span>
          ))}
        </div>
      )}

      <CodeMirror
        ref={cmRef}
        value={content}
        height="100%"
        theme="light"
        extensions={[
          markdown({ base: markdownLanguage }),
          EditorView.lineWrapping,
        ]}
        editable={!isViewer}
        readOnly={isViewer}
        placeholder={isViewer ? 'You have read-only access to this document.' : 'Start writing collaborative notes in Markdown...'}
        onChange={handleChange}
        onUpdate={handleUpdate}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLineGutter: true,
          highlightSpecialChars: true,
          history: true,
          foldGutter: true,
          drawSelection: true,
          dropCursor: true,
          allowMultipleSelections: true,
          indentOnInput: true,
          syntaxHighlighting: true,
          bracketMatching: true,
          closeBrackets: true,
          autocompletion: true,
          rectangularSelection: true,
          crosshairCursor: true,
          highlightActiveLine: !isViewer,
          highlightSelectionMatches: true,
          closeBracketsKeymap: true,
          searchKeymap: true,
          foldKeymap: true,
          completionKeymap: true,
          lintKeymap: true,
        }}
      />
    </div>
  );
};

export default CodeMirrorEditor;

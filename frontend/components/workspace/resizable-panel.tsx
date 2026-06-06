'use client';

import { useState, useCallback, useRef, useEffect } from 'react';

interface ResizablePanelProps {
  left: React.ReactNode;
  right: React.ReactNode;
  defaultLeftWidth?: number;
  minLeftWidth?: number;
  maxLeftWidth?: number;
}

export function ResizablePanel({
  left, right,
  defaultLeftWidth = 260, minLeftWidth = 160, maxLeftWidth = 480
}: ResizablePanelProps) {
  const [leftWidth, setLeftWidth] = useState(defaultLeftWidth);
  const dragging = useRef(false);

  const onMouseDown = useCallback(() => {
    dragging.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, []);

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!dragging.current) return;
      const newWidth = Math.max(minLeftWidth, Math.min(maxLeftWidth, e.clientX));
      setLeftWidth(newWidth);
    };
    const onMouseUp = () => {
      if (dragging.current) {
        dragging.current = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      }
    };
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, [minLeftWidth, maxLeftWidth]);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div style={{ width: leftWidth, minWidth: leftWidth }} className="flex-shrink-0 overflow-hidden">
        {left}
      </div>
      <div
        onMouseDown={onMouseDown}
        className="w-1 bg-black cursor-col-resize flex-shrink-0 hover:bg-brutal-pink transition-colors active:bg-brutal-pink"
        role="separator"
        aria-orientation="vertical"
      />
      <div className="flex-1 overflow-hidden">
        {right}
      </div>
    </div>
  );
}

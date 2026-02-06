import { AudioRecorder, useAudioRecorder } from 'react-audio-voice-recorder';

import { Button } from '@/components/ui/button';
import { Authorization } from '@/constants/authorization';
import { cn } from '@/lib/utils';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { Loader2, Mic, Square } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useIsDarkTheme } from '../theme-provider';
import { Input } from './input';
import { Popover, PopoverContent, PopoverTrigger } from './popover';
const VoiceVisualizer = ({ isRecording }: { isRecording: boolean }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const animationFrameRef = useRef<number>(0);
  const streamRef = useRef<MediaStream | null>(null);
  const isDark = useIsDarkTheme();

  const startVisualization = async () => {
    try {
      // Check if the browser supports getUserMedia
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        console.error('Browser does not support getUserMedia API');
        return;
      }
      // Request microphone permission
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;

      // Create audio context and analyzer
      const audioContext = new (window.AudioContext ||
        (window as any).webkitAudioContext)();
      audioContextRef.current = audioContext;

      const analyser = audioContext.createAnalyser();
      analyserRef.current = analyser;
      analyser.fftSize = 32;

      // Connect audio nodes
      const source = audioContext.createMediaStreamSource(stream);
      source.connect(analyser);

      // Start drawing
      draw();
    } catch (error) {
      console.error(
        'Unable to access microphone for voice visualization:',
        error,
      );
    }
  };

  const stopVisualization = () => {
    // Stop animation frame
    if (animationFrameRef.current) {
      cancelAnimationFrame(animationFrameRef.current);
    }

    // Stop audio stream
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((track) => track.stop());
    }

    // Close audio context
    if (audioContextRef.current && audioContextRef.current.state !== 'closed') {
      audioContextRef.current.close();
    }

    // Clear canvas
    const canvas = canvasRef.current;
    if (canvas) {
      const ctx = canvas.getContext('2d');
      if (ctx) {
        ctx.clearRect(0, 0, canvas.width, canvas.height);
      }
    }
  };
  useEffect(() => {
    if (isRecording) {
      startVisualization();
    } else {
      stopVisualization();
    }

    return () => {
      stopVisualization();
    };
  }, [isRecording]);
  const draw = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const analyser = analyserRef.current;
    if (!analyser) return;

    // Set canvas dimensions
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;
    const centerY = height / 2;

    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width;
      canvas.height = height;
    }

    // Clear canvas
    ctx.clearRect(0, 0, width, height);

    // Get frequency data
    const bufferLength = analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);
    analyser.getByteFrequencyData(dataArray);

    // Draw waveform
    const barWidth = (width / bufferLength) * 1.5;
    let x = 0;

    for (let i = 0; i < bufferLength; i = i + 2) {
      const barHeight = (dataArray[i] / 255) * centerY;

      // Create gradient
      const gradient = ctx.createLinearGradient(
        0,
        centerY - barHeight,
        0,
        centerY + barHeight,
      );
      gradient.addColorStop(0, '#3ba05c'); // Blue
      gradient.addColorStop(1, '#3ba05c'); // Light blue
      // gradient.addColorStop(0, isDark ? '#fff' : '#000'); // Blue
      // gradient.addColorStop(1, isDark ? '#eee' : '#eee'); // Light blue

      ctx.fillStyle = gradient;
      ctx.fillRect(x, centerY - barHeight, barWidth, barHeight * 2);

      x += barWidth + 2;
    }

    animationFrameRef.current = requestAnimationFrame(draw);
  };

  return (
    <div className="w-full h-6 bg-transparent flex items-center justify-center overflow-hidden ">
      <canvas ref={canvasRef} className="w-full h-full" />
    </div>
  );
};

const VoiceInputBox = ({
  isRecording,
  onStop,
  recordingTime,
  value,
}: {
  value: string;
  isRecording: boolean;
  onStop: () => void;
  recordingTime: number;
}) => {
  // Format recording time
  const formatTime = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  return (
    <div className="w-full">
      <div className=" absolute w-full h-6 translate-y-full">
        <VoiceVisualizer isRecording={isRecording} />
      </div>
      <Input
        rootClassName="w-full"
        className="flex-1 "
        readOnly
        value={value}
        suffix={
          <div className="flex justify-end px-1 items-center gap-1 w-20">
            <Button
              variant={'ghost'}
              size="sm"
              className="text-text-primary p-1 border-none hover:bg-transparent"
              onClick={onStop}
            >
              <Square className="text-text-primary" size={12} />
            </Button>
            <span className="text-xs text-text-secondary">
              {formatTime(recordingTime)}
            </span>
          </div>
        }
      />
    </div>
  );
};
export const AudioButton = ({
  onOk,
}: {
  onOk?: (transcript: string) => void;
}) => {
  // const [showInputBox, setShowInputBox] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [recordingTime, setRecordingTime] = useState(0);
  const [transcript, setTranscript] = useState('');
  const [popoverOpen, setPopoverOpen] = useState(false);
  const recorderControls = useAudioRecorder();
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  // Handle logic after recording is complete
  const handleRecordingComplete = async (blob: Blob) => {
    setIsRecording(false);

    // const url = URL.createObjectURL(blob);
    // const a = document.createElement('a');
    // a.href = url;
    // a.download = 'recording.webm';
    // document.body.appendChild(a);
    // a.click();

    setIsProcessing(true);
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    try {
      const audioFile = new File([blob], 'recording.webm', {
        type: blob.type || 'audio/webm',
        // type: 'audio/mpeg',
      });

      const formData = new FormData();
      formData.append('file', audioFile);
      formData.append('stream', 'false');

      const response = await fetch(api.sequence2txt, {
        method: 'POST',
        headers: {
          [Authorization]: getAuthorization(),
          // 'Content-Type': blob.type || 'audio/webm',
        },
        body: formData,
      });

      // if (!response.ok) {
      //   throw new Error(`HTTP error! status: ${response.status}`);
      // }

      // if (!response.body) {
      //   throw new Error('ReadableStream not supported in this browser');
      // }

      console.log('Response:', response);
      const { data, code } = await response.json();
      if (code === 0 && data && data.text) {
        setTranscript(data.text);
        console.log('Transcript:', data.text);
        onOk?.(data.text);
      }
      setPopoverOpen(false);
    } catch (error) {
      console.error('Failed to process audio:', error);
      // setTranscript(t('voiceRecorder.processingError'));
    } finally {
      setIsProcessing(false);
    }
  };

  //  Start recording
  const startRecording = () => {
    recorderControls.startRecording();
    setIsRecording(true);
    // setShowInputBox(true);
    setPopoverOpen(true);
    setRecordingTime(0);

    // Start timing
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }
    intervalRef.current = setInterval(() => {
      setRecordingTime((prev) => prev + 1);
    }, 1000);
  };

  // Stop recording
  const stopRecording = () => {
    recorderControls.stopRecording();
    setIsRecording(false);
    // setShowInputBox(false);
    setPopoverOpen(false);
    setRecordingTime(0);

    // Clear timer
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  };

  //  Clear transcription content
  // const clearTranscript = () => {
  //   setTranscript('');
  // };

  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);
  return (
    <div>
      {false && (
        <div className="flex flex-col items-center space-y-4">
          <div className="relative">
            <Popover
              open={popoverOpen}
              onOpenChange={(open) => {
                setPopoverOpen(true);
              }}
            >
              <PopoverTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    if (isRecording) {
                      stopRecording();
                    } else {
                      startRecording();
                    }
                  }}
                  className={`w-6 h-6 p-2 rounded-full border-none bg-transparent hover:bg-transparent ${
                    isRecording ? 'animate-pulse' : ''
                  }`}
                  disabled={isProcessing}
                >
                  <Mic size={16} className="text-text-primary" />
                </Button>
              </PopoverTrigger>
              <PopoverContent
                align="end"
                sideOffset={-20}
                className="p-0 border-none"
              >
                <VoiceInputBox
                  isRecording={isRecording}
                  value={transcript}
                  onStop={stopRecording}
                  recordingTime={recordingTime}
                />
              </PopoverContent>
            </Popover>
          </div>
        </div>
      )}

      <div className=" relative w-6 h-6 flex items-center justify-center">
        {isRecording && (
          <div
            className={cn(
              'absolute inset-0 w-full h-6 rounded-full overflow-hidden flex items-center justify-center p-1',
              { 'bg-state-success-5': isRecording },
            )}
          >
            <VoiceVisualizer isRecording={isRecording} />
          </div>
        )}
        {isRecording && (
          <div className="absolute inset-0 rounded-full border-2 border-state-success animate-ping opacity-75"></div>
        )}
        <Button
          variant="outline"
          size="sm"
          // onMouseDown={() => {
          //   startRecording();
          // }}
          // onMouseUp={() => {
          //   stopRecording();
          // }}
          onClick={() => {
            if (isRecording) {
              stopRecording();
            } else {
              startRecording();
            }
          }}
          className={`w-6 h-6 p-2 rounded-md border-none bg-transparent hover:bg-state-success-5 ${
            isRecording
              ? 'animate-pulse bg-state-success-5 text-state-success'
              : ''
          }`}
          disabled={isProcessing}
        >
          {isProcessing ? (
            <Loader2 size={16} className=" animate-spin" />
          ) : isRecording ? (
            <></>
          ) : (
            // <Mic size={16} className="text-text-primary" />
            // <Square size={12} className="text-text-primary" />
            <Mic size={16} />
          )}
        </Button>
      </div>

      {/* Hide original component */}
      <div className="hidden">
        <AudioRecorder
          onRecordingComplete={handleRecordingComplete}
          recorderControls={recorderControls}
        />
      </div>
    </div>
  );
};

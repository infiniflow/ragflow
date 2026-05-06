import CopyToClipboard from '@/components/copy-to-clipboard';
import PdfSheet from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { MessageType, SharedFrom } from '@/constants/chat';
import { useFetchExternalAgentInputs } from '@/hooks/use-agent-request';
import { useFetchExternalChatInfo } from '@/hooks/use-chat-request';
import i18n, { changeLanguageAsync } from '@/locales/config';
import { useSendNextSharedMessage } from '@/pages/agent/hooks/use-send-shared-message';
import { MessageCircle, Minimize2, Send, X } from 'lucide-react';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useGetSharedChatSearchParams,
  useSendSharedMessage,
} from '../pages/next-chats/hooks/use-send-shared-message';
import FloatingChatWidgetMarkdown from './floating-chat-widget-markdown';

/**
 * Normalizes a hex color input and falls back to a safe default when invalid.
 */
const normalizeHexColor = (value: string | null, fallback: string) => {
  return value && /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(value)
    ? value
    : fallback;
};

/**
 * Darkens a hex color to derive hover and gradient variants for the widget chrome.
 */
const darkenHexColor = (hexColor: string, amount = 0.12) => {
  const normalizedHex = hexColor.replace('#', '');
  const expandedHex =
    normalizedHex.length === 3
      ? normalizedHex
          .split('')
          .map((char) => `${char}${char}`)
          .join('')
      : normalizedHex;
  const channels = expandedHex.match(/.{2}/g);

  if (!channels) {
    return hexColor;
  }

  return `#${channels
    .map((channel) => {
      const value = parseInt(channel, 16);
      const adjustedValue = Math.max(
        0,
        Math.min(255, Math.round(value * (1 - amount))),
      );
      return adjustedValue.toString(16).padStart(2, '0');
    })
    .join('')}`;
};

/**
 * Accepts a footer link from the widget query string and returns a safe HTTP(S) URL.
 */
const normalizeWidgetFooterLink = (value: string | null) => {
  const normalizedValue = value?.trim();

  if (!normalizedValue) {
    return undefined;
  }

  const candidate = /^[a-z][a-z\d+.-]*:/i.test(normalizedValue)
    ? normalizedValue
    : `https://${normalizedValue}`;

  try {
    const url = new URL(candidate);

    if (url.protocol === 'http:' || url.protocol === 'https:') {
      return url.toString();
    }
  } catch {
    return undefined;
  }

  return undefined;
};

/**
 * Renders the embeddable floating chat widget and applies URL-driven widget settings.
 */
const FloatingChatWidget = () => {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const [isMinimized, setIsMinimized] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [lastResponseId, setLastResponseId] = useState<string | null>(null);
  const [displayMessages, setDisplayMessages] = useState<any[]>([]);
  const [isLoaded, setIsLoaded] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const {
    sharedId: conversationId,
    locale,
    from,
  } = useGetSharedChatSearchParams();

  const isFromAgent = from === SharedFrom.Agent;

  // Check if we're in button-only mode or window-only mode
  const urlParams = new URLSearchParams(window.location.search);
  const mode = urlParams.get('mode') || 'full'; // 'button', 'window', or 'full'
  const enableStreaming = urlParams.get('streaming') === 'true'; // Only enable if explicitly set to true
  const isMuted = urlParams.get('muted') === 'true';
  const widgetTitle = urlParams.get('widget_title')?.trim();
  const widgetSubtitle = urlParams.get('widget_subtitle')?.trim();
  const widgetFooter = urlParams.get('widget_footer')?.trim();
  const widgetFooterLink = normalizeWidgetFooterLink(
    urlParams.get('widget_footer_link'),
  );
  const widgetAccentColor = normalizeHexColor(
    urlParams.get('widget_accent_color'),
    '#2563eb',
  );
  const widgetAccentColorStrong = darkenHexColor(widgetAccentColor);
  const widgetBackgroundColor = normalizeHexColor(
    urlParams.get('widget_background_color'),
    '#ffffff',
  );
  const widgetTextColor = normalizeHexColor(
    urlParams.get('widget_text_color'),
    '#111827',
  );
  const widgetHeaderTextColor = normalizeHexColor(
    urlParams.get('widget_header_text_color'),
    '#ffffff',
  );
  const widgetFooterTextColor = normalizeHexColor(
    urlParams.get('widget_footer_text_color'),
    '#111827',
  );

  const {
    handlePressEnter,
    handleInputChange,
    value: hookValue,
    sendLoading,
    derivedMessages,
    hasError,
  } = (isFromAgent ? useSendNextSharedMessage : useSendSharedMessage)(() => {});

  // Sync our local input with the hook's value when needed
  useEffect(() => {
    if (hookValue && hookValue !== inputValue) {
      setInputValue(hookValue);
    }
  }, [hookValue, inputValue]);

  const { data } = (
    isFromAgent ? useFetchExternalAgentInputs : useFetchExternalChatInfo
  )();

  const title = data.title;
  const displayTitle = widgetTitle || title || t('chat.chatSupport');
  const displaySubtitle = widgetSubtitle || t('chat.replyInstantly');
  const displayFooter = widgetFooter || '';
  const renderFooter = () => {
    if (!displayFooter) {
      return null;
    }

    return (
      <div
        className="mt-3 -mx-4 -mb-4 px-4 py-3 text-center text-xs"
        style={{
          backgroundColor: widgetAccentColor,
          color: widgetFooterTextColor,
        }}
      >
        {widgetFooterLink ? (
          <a
            href={widgetFooterLink}
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-2 hover:opacity-90"
            style={{ color: widgetFooterTextColor }}
          >
            {displayFooter}
          </a>
        ) : (
          displayFooter
        )}
      </div>
    );
  };
  const bodyContainerStyle: React.CSSProperties = {
    borderRadius: '0 0 16px 16px',
    backgroundColor: widgetBackgroundColor,
    color: widgetTextColor,
  };
  const inputStyle: React.CSSProperties = {
    minHeight: '44px',
    maxHeight: '120px',
    color: widgetTextColor,
    backgroundColor: widgetBackgroundColor,
  };

  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  // PDF drawer state tracking
  useEffect(() => {
    // Drawer state management
  }, [visible, documentId, selectedChunk]);

  // Play sound when opening
  const playNotificationSound = useCallback(() => {
    if (isMuted) {
      return;
    }

    try {
      const audioContext = new (
        window.AudioContext || (window as any).webkitAudioContext
      )();
      const oscillator = audioContext.createOscillator();
      const gainNode = audioContext.createGain();

      oscillator.connect(gainNode);
      gainNode.connect(audioContext.destination);

      oscillator.frequency.value = 800;
      oscillator.type = 'sine';

      gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
      gainNode.gain.exponentialRampToValueAtTime(
        0.01,
        audioContext.currentTime + 0.3,
      );

      oscillator.start(audioContext.currentTime);
      oscillator.stop(audioContext.currentTime + 0.3);
    } catch (error) {
      console.warn(error);
      // Silent fail if audio not supported
    }
  }, [isMuted]);

  // Play sound for AI responses (Intercom-style)
  const playResponseSound = useCallback(() => {
    if (isMuted) {
      return;
    }

    try {
      const audioContext = new (
        window.AudioContext || (window as any).webkitAudioContext
      )();
      const oscillator = audioContext.createOscillator();
      const gainNode = audioContext.createGain();

      oscillator.connect(gainNode);
      gainNode.connect(audioContext.destination);

      oscillator.frequency.value = 600;
      oscillator.type = 'sine';

      gainNode.gain.setValueAtTime(0.2, audioContext.currentTime);
      gainNode.gain.exponentialRampToValueAtTime(
        0.01,
        audioContext.currentTime + 0.2,
      );

      oscillator.start(audioContext.currentTime);
      oscillator.stop(audioContext.currentTime + 0.2);
    } catch (error) {
      console.warn(error);

      // Silent fail if audio not supported
    }
  }, [isMuted]);

  // Set loaded state and locale
  useEffect(() => {
    // Set component as loaded after a brief moment to prevent flash
    const timer = setTimeout(() => {
      setIsLoaded(true);
      // Tell parent window that we're ready to be shown
      window.parent.postMessage(
        {
          type: 'WIDGET_READY',
        },
        '*',
      );
    }, 50);

    if (locale && i18n.language !== locale) {
      changeLanguageAsync(locale);
    }

    return () => clearTimeout(timer);
  }, [locale]);

  // Handle message display based on streaming preference
  useEffect(() => {
    if (!derivedMessages) {
      setDisplayMessages([]);
      return;
    }

    if (enableStreaming) {
      // Show messages as they stream
      setDisplayMessages(derivedMessages);
    } else {
      // Only show complete messages (non-streaming mode)
      const completeMessages = derivedMessages.filter((msg, index) => {
        // Always show user messages immediately
        if (msg.role === MessageType.User) return true;

        // For AI messages, only show when response is complete (not loading)
        if (msg.role === MessageType.Assistant) {
          return !sendLoading || index < derivedMessages.length - 1;
        }

        return true;
      });
      setDisplayMessages(completeMessages);
    }
  }, [derivedMessages, enableStreaming, sendLoading]);

  // Auto-scroll to bottom when display messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [displayMessages]);

  // Render different content based on mode
  // Master mode - handles everything and creates second iframe dynamically
  useEffect(() => {
    if (mode !== 'master') return;

    const isInIframe = window.self !== window.top;

    if (isInIframe) {
      // Embedded: tell parent to create chat window iframe
      window.parent.postMessage(
        {
          type: 'CREATE_CHAT_WINDOW',
          src: window.location.href.replace('mode=master', 'mode=window'),
        },
        '*',
      );
    } else {
      // Standalone: create chat window iframe ourselves
      if (!document.getElementById('chat-win')) {
        const i = document.createElement('iframe');
        i.id = 'chat-win';
        i.src = window.location.href.replace('mode=master', 'mode=window');
        i.style.cssText =
          'position:fixed;bottom:104px;right:24px;width:380px;height:500px;border:none;background:transparent;z-index:9998;display:none';
        i.frameBorder = '0';
        i.allow = 'microphone;camera';
        document.body.appendChild(i);
      }
    }

    // Listen for toggle messages to show/hide the chat window iframe
    const handleToggle = (e: MessageEvent) => {
      if (e.data.type === 'TOGGLE_CHAT') {
        const chatWindow = document.getElementById(
          'chat-win',
        ) as HTMLIFrameElement;
        if (chatWindow) {
          chatWindow.style.display = e.data.isOpen ? 'block' : 'none';
        }
      }
    };

    window.addEventListener('message', handleToggle);
    return () => window.removeEventListener('message', handleToggle);
  }, [mode]);

  // Play sound only when AI response is complete (not streaming chunks)
  useEffect(() => {
    if (derivedMessages && derivedMessages.length > 0 && !sendLoading) {
      const lastMessage = derivedMessages[derivedMessages.length - 1];
      if (
        lastMessage.role === MessageType.Assistant &&
        lastMessage.id !== lastResponseId &&
        derivedMessages.length > 1
      ) {
        setLastResponseId(lastMessage.id || '');
        playResponseSound();
      }
    }
  }, [derivedMessages, sendLoading, lastResponseId, playResponseSound]);

  const toggleChat = useCallback(() => {
    if (mode === 'button') {
      // In button mode, communicate with parent window to show/hide chat window
      window.parent.postMessage(
        {
          type: 'TOGGLE_CHAT',
          isOpen: !isOpen,
        },
        '*',
      );
      setIsOpen(!isOpen);
      if (!isOpen) {
        playNotificationSound();
      }
    } else {
      // In full mode, handle locally
      if (!isOpen) {
        setIsOpen(true);
        setIsMinimized(false);
        playNotificationSound();
      } else {
        setIsOpen(false);
        setIsMinimized(false);
      }
    }
  }, [isOpen, mode, playNotificationSound]);

  const minimizeChat = useCallback(() => {
    setIsMinimized(true);
  }, []);

  const handleSendMessage = useCallback(() => {
    if (!inputValue.trim() || sendLoading) return;

    // Update the hook's internal state first
    const syntheticEvent = {
      target: { value: inputValue },
      currentTarget: { value: inputValue },
      preventDefault: () => {},
    } as any;

    handleInputChange(syntheticEvent);

    // Wait for state to update, then send
    setTimeout(() => {
      handlePressEnter({ enableThinking: false, enableInternet: false });
      // Clear our local input after sending
      setInputValue('');
    }, 50);
  }, [inputValue, sendLoading, handleInputChange, handlePressEnter]);

  const handleKeyPress = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSendMessage();
      }
    },
    [handleSendMessage],
  );

  if (!conversationId) {
    return (
      <div className="fixed bottom-5 right-5 z-50">
        <div className="bg-red-500 text-white p-4 rounded-lg shadow-lg">
          Error: No conversation ID provided
        </div>
      </div>
    );
  }

  // Remove the blocking return - we'll handle visibility with CSS instead

  const messageCount = displayMessages?.length || 0;

  // Show just the button in master mode
  if (mode === 'master') {
    return (
      <div
        className={`fixed bottom-6 right-6 z-50 transition-opacity duration-300 ${isLoaded ? 'opacity-100' : 'opacity-0'}`}
      >
        <button
          type="button"
          onClick={() => {
            const newIsOpen = !isOpen;
            setIsOpen(newIsOpen);
            if (newIsOpen) playNotificationSound();

            // Send toggle message to parent (if embedded) or self (if standalone)
            const target = window.self !== window.top ? window.parent : window;
            target.postMessage(
              {
                type: 'TOGGLE_CHAT',
                isOpen: newIsOpen,
              },
              '*',
            );
          }}
          className={`w-14 h-14 text-white rounded-full transition-all duration-300 flex items-center justify-center group ${
            isOpen ? 'scale-95' : 'scale-100 hover:scale-105'
          }`}
          style={{ backgroundColor: widgetAccentColor }}
        >
          <div
            className={`transition-transform duration-300 ${isOpen ? 'rotate-45' : 'rotate-0'}`}
          >
            {isOpen ? <X size={24} /> : <MessageCircle size={24} />}
          </div>
        </button>

        {/* Unread Badge */}
        {!isOpen && messageCount > 0 && (
          <div className="absolute -top-2 -right-2 w-6 h-6 bg-red-500 text-white text-xs font-bold rounded-full flex items-center justify-center animate-pulse">
            {messageCount > 9 ? '9+' : messageCount}
          </div>
        )}
      </div>
    );
  }

  if (mode === 'button') {
    // Only render the floating button
    return (
      <div
        className={`fixed bottom-6 right-6 z-50 transition-opacity duration-300 ${isLoaded ? 'opacity-100' : 'opacity-0'}`}
      >
        <button
          type="button"
          onClick={toggleChat}
          className={`w-14 h-14 text-white rounded-full transition-all duration-300 flex items-center justify-center group ${
            isOpen ? 'scale-95' : 'scale-100 hover:scale-105'
          }`}
          style={{ backgroundColor: widgetAccentColor }}
        >
          <div
            className={`transition-transform duration-300 ${isOpen ? 'rotate-45' : 'rotate-0'}`}
          >
            {isOpen ? <X size={24} /> : <MessageCircle size={24} />}
          </div>
        </button>

        {/* Unread Badge */}
        {!isOpen && messageCount > 0 && (
          <div className="absolute -top-2 -right-2 w-6 h-6 bg-red-500 text-white text-xs font-bold rounded-full flex items-center justify-center animate-pulse">
            {messageCount > 9 ? '9+' : messageCount}
          </div>
        )}
      </div>
    );
  }

  if (mode === 'window') {
    // Only render the chat window (always open)
    return (
      <>
        <div
          className={`fixed top-0 left-0 z-50 rounded-2xl transition-all duration-300 ease-out h-[500px] w-[380px] overflow-hidden ${isLoaded ? 'opacity-100' : 'opacity-0'}`}
          style={{ backgroundColor: widgetAccentColor }}
        >
          {/* Header */}
          <div
            className="flex items-center justify-between p-4 text-white rounded-t-2xl"
            style={{
              background: `linear-gradient(to right, ${widgetAccentColor}, ${widgetAccentColorStrong})`,
            }}
          >
            <div className="flex items-center space-x-3">
              <div className="w-8 h-8 bg-white bg-opacity-20 rounded-full flex items-center justify-center">
                <MessageCircle size={18} />
              </div>
              <div>
                <h3
                  className="font-semibold text-sm"
                  style={{ color: widgetHeaderTextColor }}
                >
                  {displayTitle}
                </h3>
                <p className="text-xs" style={{ color: widgetHeaderTextColor }}>
                  {displaySubtitle}
                </p>
              </div>
            </div>
          </div>

          {/* Messages and Input */}
          <div className="flex flex-col h-[436px]" style={bodyContainerStyle}>
            <div
              className="flex-1 overflow-y-auto p-4 space-y-4"
              onWheel={(e) => {
                const element = e.currentTarget;
                const isAtTop = element.scrollTop === 0;
                const isAtBottom =
                  element.scrollTop + element.clientHeight >=
                  element.scrollHeight - 1;

                // Allow scroll to pass through to parent when at boundaries
                if ((isAtTop && e.deltaY < 0) || (isAtBottom && e.deltaY > 0)) {
                  e.preventDefault();
                  // Let the parent handle the scroll
                  window.parent.postMessage(
                    {
                      type: 'SCROLL_PASSTHROUGH',
                      deltaY: e.deltaY,
                    },
                    '*',
                  );
                }
              }}
            >
              {displayMessages?.map((message, index) => (
                <div
                  key={index}
                  className={`flex ${message.role === MessageType.User ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`group max-w-[280px] px-4 py-2 rounded-2xl ${
                      message.role === MessageType.User
                        ? 'text-white rounded-br-md'
                        : 'rounded-bl-md'
                    }`}
                    style={
                      message.role === MessageType.User
                        ? { backgroundColor: widgetAccentColor }
                        : {
                            backgroundColor: 'rgba(148, 163, 184, 0.14)',
                            color: widgetTextColor,
                          }
                    }
                  >
                    {message.role === MessageType.User ? (
                      <p className="text-sm leading-relaxed whitespace-pre-wrap">
                        {message.content}
                      </p>
                    ) : (
                      <div className="space-y-2">
                        <FloatingChatWidgetMarkdown
                          loading={false}
                          content={message.content}
                          reference={
                            message.reference || {
                              doc_aggs: [],
                              chunks: [],
                              total: 0,
                            }
                          }
                          clickDocumentButton={clickDocumentButton}
                        />
                        <div
                          className="flex justify-end opacity-0 transition-opacity group-hover:opacity-100"
                          role="toolbar"
                        >
                          <CopyToClipboard
                            text={message.content}
                            className="border-0"
                            size="icon-xs"
                          />
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              ))}

              {/* Clean Typing Indicator */}
              {sendLoading && !enableStreaming && (
                <div className="flex justify-start pl-4">
                  <div className="flex space-x-1">
                    <div
                      className="w-2 h-2 rounded-full animate-bounce"
                      style={{ backgroundColor: widgetAccentColor }}
                    ></div>
                    <div
                      className="w-2 h-2 rounded-full animate-bounce"
                      style={{
                        backgroundColor: widgetAccentColor,
                        animationDelay: '0.1s',
                      }}
                    ></div>
                    <div
                      className="w-2 h-2 rounded-full animate-bounce"
                      style={{
                        backgroundColor: widgetAccentColor,
                        animationDelay: '0.2s',
                      }}
                    ></div>
                  </div>
                </div>
              )}

              <div ref={messagesEndRef} />
            </div>

            {/* Input Area */}
            <div
              className="border-t border-gray-200 p-4"
              style={bodyContainerStyle}
            >
              <div className="flex items-end space-x-3">
                <div className="flex-1">
                  <textarea
                    value={inputValue}
                    onChange={(e) => {
                      const newValue = e.target.value;
                      setInputValue(newValue);
                      handleInputChange(e);
                    }}
                    onKeyPress={handleKeyPress}
                    placeholder={t('chat.typeYourMessage')}
                    rows={1}
                    className="w-full resize-none border border-gray-300 rounded-2xl px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:border-transparent"
                    style={inputStyle}
                    disabled={hasError || sendLoading}
                  />
                </div>
                <button
                  type="button"
                  onClick={handleSendMessage}
                  disabled={!inputValue.trim() || sendLoading}
                  className="p-3 text-white rounded-full disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  style={{ backgroundColor: widgetAccentColor }}
                >
                  <Send size={18} />
                </button>
              </div>
              {renderFooter()}
            </div>
          </div>
        </div>
        {visible && (
          <PdfSheet
            visible={visible}
            hideModal={hideModal}
            documentId={documentId}
            chunk={selectedChunk}
            width={'100vw'}
            height={'100vh'}
          />
        )}
      </>
    );
  } // Full mode - render everything together (original behavior)
  return (
    <div
      className={`transition-opacity duration-300 ${isLoaded ? 'opacity-100' : 'opacity-0'}`}
    >
      {/* Chat Widget Container */}
      {isOpen && (
        <div
          className={`fixed bottom-24 right-6 z-50 rounded-2xl transition-all duration-300 ease-out ${
            isMinimized ? 'h-16' : 'h-[500px]'
          } w-[380px] overflow-hidden`}
          style={{ backgroundColor: widgetAccentColor }}
        >
          {/* Header */}
          <div
            className="flex items-center justify-between p-4 text-white rounded-t-2xl"
            style={{
              background: `linear-gradient(to right, ${widgetAccentColor}, ${widgetAccentColorStrong})`,
            }}
          >
            <div className="flex items-center space-x-3">
              <div className="w-8 h-8 bg-white bg-opacity-20 rounded-full flex items-center justify-center">
                <MessageCircle size={18} />
              </div>
              <div>
                <h3
                  className="font-semibold text-sm"
                  style={{ color: widgetHeaderTextColor }}
                >
                  {displayTitle}
                </h3>
                <p className="text-xs" style={{ color: widgetHeaderTextColor }}>
                  {displaySubtitle}
                </p>
              </div>
            </div>
            <div className="flex items-center space-x-1">
              <button
                type="button"
                onClick={minimizeChat}
                className="p-1.5 hover:bg-white hover:bg-opacity-20 rounded-full transition-colors"
              >
                <Minimize2 size={16} />
              </button>
              <button
                type="button"
                onClick={toggleChat}
                className="p-1.5 hover:bg-white hover:bg-opacity-20 rounded-full transition-colors"
              >
                <X size={16} />
              </button>
            </div>
          </div>

          {/* Messages Container */}
          {!isMinimized && (
            <div className="flex flex-col h-[436px]" style={bodyContainerStyle}>
              <div
                className="flex-1 overflow-y-auto p-4 space-y-4"
                onWheel={(e) => {
                  const element = e.currentTarget;
                  const isAtTop = element.scrollTop === 0;
                  const isAtBottom =
                    element.scrollTop + element.clientHeight >=
                    element.scrollHeight - 1;

                  // Allow scroll to pass through to parent when at boundaries
                  if (
                    (isAtTop && e.deltaY < 0) ||
                    (isAtBottom && e.deltaY > 0)
                  ) {
                    e.preventDefault();
                    // Let the parent handle the scroll
                    window.parent.postMessage(
                      {
                        type: 'SCROLL_PASSTHROUGH',
                        deltaY: e.deltaY,
                      },
                      '*',
                    );
                  }
                }}
              >
                {displayMessages?.map((message, index) => (
                  <div
                    key={index}
                    className={`flex ${message.role === MessageType.User ? 'justify-end' : 'justify-start'}`}
                  >
                    <div
                      className={`group max-w-[280px] px-4 py-2 rounded-2xl ${
                        message.role === MessageType.User
                          ? 'text-white rounded-br-md'
                          : 'rounded-bl-md'
                      }`}
                      style={
                        message.role === MessageType.User
                          ? { backgroundColor: widgetAccentColor }
                          : {
                              backgroundColor: 'rgba(148, 163, 184, 0.14)',
                              color: widgetTextColor,
                            }
                      }
                    >
                      {message.role === MessageType.User ? (
                        <p className="text-sm leading-relaxed whitespace-pre-wrap">
                          {message.content}
                        </p>
                      ) : (
                        <div className="space-y-2">
                          <FloatingChatWidgetMarkdown
                            loading={false}
                            content={message.content}
                            reference={
                              message.reference || {
                                doc_aggs: [],
                                chunks: [],
                                total: 0,
                              }
                            }
                            clickDocumentButton={clickDocumentButton}
                          />
                          <div
                            className="flex justify-end opacity-0 transition-opacity group-hover:opacity-100"
                            role="toolbar"
                          >
                            <CopyToClipboard
                              text={message.content}
                              className="border-0"
                              size="icon-xs"
                            />
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                ))}

                {/* Typing Indicator */}
                {sendLoading && (
                  <div className="flex justify-start">
                    <div className="bg-gray-100 rounded-2xl rounded-bl-md px-4 py-3">
                      <div className="flex space-x-1">
                        <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"></div>
                        <div
                          className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
                          style={{ animationDelay: '0.1s' }}
                        ></div>
                        <div
                          className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
                          style={{ animationDelay: '0.2s' }}
                        ></div>
                      </div>
                    </div>
                  </div>
                )}

                <div ref={messagesEndRef} />
              </div>

              {/* Input Area */}
              <div className="border-t border-gray-200 p-4">
                <div className="flex items-end space-x-3">
                  <div className="flex-1">
                    <textarea
                      value={inputValue}
                      onChange={(e) => {
                        const newValue = e.target.value;
                        setInputValue(newValue);
                        // Also update the hook's state
                        handleInputChange(e);
                      }}
                      onKeyPress={handleKeyPress}
                      placeholder={t('chat.typeYourMessage')}
                      rows={1}
                      className="w-full resize-none border border-gray-300 rounded-2xl px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:border-transparent"
                      style={inputStyle}
                      disabled={hasError || sendLoading}
                    />
                  </div>
                  <button
                    type="button"
                    onClick={handleSendMessage}
                    disabled={!inputValue.trim() || sendLoading}
                    className="p-3 text-white rounded-full disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                    style={{ backgroundColor: widgetAccentColor }}
                  >
                    <Send size={18} />
                  </button>
                </div>
                {renderFooter()}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Floating Button */}
      <div className="fixed bottom-6 right-6 z-50">
        <button
          type="button"
          onClick={toggleChat}
          className={`w-14 h-14 text-white rounded-full transition-all duration-300 flex items-center justify-center group ${
            isOpen ? 'scale-95' : 'scale-100 hover:scale-105'
          }`}
          style={{ backgroundColor: widgetAccentColor }}
        >
          <div
            className={`transition-transform duration-300 ${isOpen ? 'rotate-45' : 'rotate-0'}`}
          >
            {isOpen ? <X size={24} /> : <MessageCircle size={24} />}
          </div>
        </button>

        {/* Unread Badge */}
        {!isOpen && messageCount > 0 && (
          <div className="absolute -top-2 -right-2 w-6 h-6 bg-red-500 text-white text-xs font-bold rounded-full flex items-center justify-center animate-pulse">
            {messageCount > 9 ? '9+' : messageCount}
          </div>
        )}
      </div>
      <PdfSheet
        visible={visible}
        hideModal={hideModal}
        documentId={documentId}
        chunk={selectedChunk}
      />
    </div>
  );
};

export default FloatingChatWidget;

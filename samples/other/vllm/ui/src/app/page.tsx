'use client';

import { useChat } from 'ai/react';
import { useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';

interface MessageProps {
  text: string;
  isBot: boolean;
}

function Message({
  text,
  isBot
}: MessageProps) {
  return (
    <div className={`chat ${isBot ? 'chat-start' : 'chat-end'}`}>
      <div className={`chat-bubble ${isBot ? 'bg-purple-700' : 'bg-blue-500'} shadow-xl`}>
        <ReactMarkdown>{text}</ReactMarkdown>
      </div>
    </div>
  );
}

export default function Home() {
  const { messages, input, handleInputChange, handleSubmit } = useChat();
  const formRef = useRef<HTMLFormElement>(null);

  return (
    <main className="flex bg-gradient-defang flex-col justify-center min-h-full">
      <div className="flex-1 flex flex-col justify-between max-w-4xl w-full m-auto">
        <div className="flex-1 z-0">
          <div className="flex flex-col space-y-4 p-4">
            {messages.map((message, index) => (
              <Message key={index} text={message.content} isBot={message.role !== 'user'} />
            ))}
            <div className='w-full' style={{ height: formRef.current?.offsetHeight }} />
          </div>
        </div>
        <div className="fixed bottom-0 left-0 right-0 z-10">
          <form className="flex space-x-4 p-4 w-full max-w-4xl m-auto" onSubmit={handleSubmit} ref={formRef} >
            <textarea
              placeholder="Type a message"
              className="flex-1 p-4 rounded-xl shadow-xl overflow-auto resize-none"
              onChange={handleInputChange}
              rows={1}
              value={input}
              onInput={(event) => {
                const target = event.target as HTMLTextAreaElement;
                target.style.height = 'inherit';
                const height = target.scrollHeight;
                target.style.height = `${Math.min(height, 300)}px`;
              }}
            />
            <div className='flex flex-col justify-end'><button className="p-4 bg-purple-800 rounded-xl text-white shadow-xl hover:shadow-2xl transition-all h-auto">Send</button></div>
          </form>
        </div>

      </div>
    </main>
  );
}
'use client';

interface MessageProps {
  text: string;
  isBot: boolean;
}

function Message({
  text,
  isBot
}: MessageProps) {
  return (
    <div className={`flex ${isBot ? 'justify-start' : 'justify-end'}`}>
      <div className={`p-4 rounded-xl ${'bg-purple-700'}`}>
        <p className="text-white">{text}</p>
      </div>
    </div>
  )
}


export default function Home() {
  return (
    <main className="flex bg-purple-900 flex-col justify-center min-h-full">
      <div className="flex-1 flex flex-col justify-between max-w-4xl w-full m-auto">
        <div className="flex-1">
          <div className="flex flex-col space-y-4 p-4">
            <Message text="Hi, I'm a vLLM instance deployed with Defang. What can I help you with?" isBot={true} />
          </div>
        </div>
        <div className="flex space-x-4 p-4">
          <input
            type="text"
            placeholder="Type a message"
            className="flex-1 p-4 rounded-xl"
          />
          <button className="p-4 bg-purple-800 rounded-xl">Send</button>
        </div>
      </div>
    </main>
  );
}

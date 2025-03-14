import React, { useState, useEffect } from 'react'

const Greeting = () => {
  const [greeting, setGreeting] = useState('')
  const firstName = 'John' // This should come from user context/auth state

  useEffect(() => {
    const updateGreeting = () => {
      const hour = new Date().getHours()
      if (hour >= 5 && hour < 12) {
        setGreeting('Good Morning')
      } else if (hour >= 12 && hour < 18) {
        setGreeting('Good Afternoon')
      } else {
        setGreeting('Good Evening')
      }
    }

    updateGreeting()
    const interval = setInterval(updateGreeting, 60000)
    return () => clearInterval(interval)
  }, [])

  return (
    <h2 className="text-2xl font-semibold text-gray-800">
      {greeting}, {firstName}
    </h2>
  )
}

export default Greeting 
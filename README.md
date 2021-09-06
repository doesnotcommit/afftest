###### Notes:

- The requirement is to send responses as json, which won't work well for non-textual responses. To keep things simple, we'll assume that all responses can be safely converted to text.
- We believe the requirement is to execute outbound requests concurrently, although it is never explicitly stated so (a maximum of 4 concurrent requests).
- Looking for ways to write simple tests for the following NFRs:
  - exceeding maximum incoming http connections (100)
  - outgoing requests concurrency (4 at a time)
  - timing out requests
  - request cancellation

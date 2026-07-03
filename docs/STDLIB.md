# funk — standard library

120 functions across 13 packages.

## (local)

- **`reverse_words`** — reverse the order of words in s  
  `reverse_words(s Str) → r Str` · *atomic · python*

## funk/std/collections

- **`contains?`** — true if the list contains the given item  
  `contains?(a List, item Any) → r Bool` · *atomic · python*
- **`count`** — number of items  
  `count(xs List) → r Num` · *composite*
- **`drop`** — list without its first n items  
  `drop(a List, n Num) → r List` · *atomic · python*
- **`first`** — first item  
  `first(a List) → r Any` · *atomic · builtin*
- **`flatten`** — flatten a list of lists by one level  
  `flatten(a List) → r List` · *atomic · python*
- **`incAcc`** — add one to the accumulator, ignoring the item (used by count via fold)  
  `incAcc(a Num, b Any) → r Num` · *composite*
- **`isEmpty?`** — true if the list is empty  
  `isEmpty?(a List) → r Bool` · *atomic · builtin*
- **`last`** — last item  
  `last(a List) → r Any` · *atomic · builtin*
- **`mean`** — arithmetic mean — sum over count, in funk  
  `mean(xs List) → r Num` · *composite*
- **`product`** — product of a list/stream of numbers  
  `product(xs List) → r Num` · *composite*
- **`reverse`** — reverse the order of a list  
  `reverse(a List) → r List` · *atomic · python*
- **`sort`** — sort a list in ascending order  
  `sort(a List) → r List` · *atomic · python*
- **`sum`** — sum of a list/stream of numbers  
  `sum(xs List) → r Num` · *composite*
- **`take`** — first n items of a list  
  `take(a List, n Num) → r List` · *atomic · python*
- **`unique`** — distinct items of a list, preserving first-seen order  
  `unique(a List) → r List` · *atomic · python*
- **`zip2`** — pairwise zip two lists into a list of [x, y] pairs  
  `zip2(a List, b List) → r List` · *atomic · python*

## funk/std/datetime

- **`addSeconds`** — add n seconds to a Unix timestamp  
  `addSeconds(t Time, n Num) → r Time` · *atomic · python*
- **`dayOfWeek`** — day of week for a timestamp (Monday=0 .. Sunday=6)  
  `dayOfWeek(t Time) → r Num` · *atomic · python*
- **`formatIso`** — format a Unix timestamp as an ISO-8601 string (UTC)  
  `formatIso(t Time) → r Str` · *atomic · python*
- **`isoWeek`** — ISO week number (1-53) for a timestamp  
  `isoWeek(t Time) → r Num` · *atomic · python*
- **`now`** — current Unix timestamp in seconds  
  `now() → r Time` · *atomic · python*
- **`parseIso`** — parse an ISO-8601 string into a Unix timestamp  
  `parseIso(a Str) → r Time` · *atomic · python*

## funk/std/encoding

- **`base64Decode`** — decode a base64 string to a UTF-8 string  
  `base64Decode(a Str) → r Str` · *atomic · python*
- **`base64Encode`** — base64-encode a UTF-8 string  
  `base64Encode(a Str) → r Str` · *atomic · python*
- **`jsonParse`** — parse a JSON string into a value  
  `jsonParse(a Str) → r Json` · *atomic · python*
- **`jsonStringify`** — serialize a value to a JSON string  
  `jsonStringify(a Json) → r Str` · *atomic · python*
- **`urlDecode`** — decode a percent-encoded URL string  
  `urlDecode(a Str) → r Str` · *atomic · python*
- **`urlEncode`** — percent-encode a string for use in a URL  
  `urlEncode(a Str) → r Str` · *atomic · python*

## funk/std/examples

- **`analyze`** — mean of the last 100 items, or 'no data' if empty  
  `analyze(xs Stream<Num>) → r Num` · *composite*
- **`applyTwice`** — apply a function to x, twice — higher-order  
  `applyTwice(f Fn, x Num) → r Num` · *composite*
- **`batchSums`** — sum of each tumbling window of 3 over 1..n (windowed stream aggregate)  
  `batchSums(n Num) → r Stream<Num>` · *composite*
- **`bump`** — add 100 when x > 5, else pass x through — proves the condition gates the add  
  `bump(x Num) → r Num` · *composite*
- **`compose2`** — apply g then f to x — two functions as values  
  `compose2(f Fn, g Fn, x Num) → r Num` · *composite*
- **`countUp`** — count up to n (returns n) — a minimal while  
  `countUp(n Num) → r Num` · *composite*
- **`createIssue`** — open a GitHub issue (illustrative) — declares what it needs  
  `createIssue(title Str) → r Json` · *atomic · python*
- **`evens`** — the first n even numbers, mapped lazily from an infinite source  
  `evens(n Num) → r Stream<Num>` · *composite*
- **`firstEvens`** — the first n even naturals, filtered from an infinite source  
  `firstEvens(n Num) → r Stream<Num>` · *composite*
- **`incThenSquare`** — square(inc(x)) via compose2  
  `incThenSquare(x Num) → r Num` · *composite*
- **`liveSquares`** — square of a tick, every 200ms, n times  
  `liveSquares(n Num) → r Stream<Num>` · *composite*
- **`mergedCount`** — how many items when two ranges are merged  
  `mergedCount(n Num) → r Num` · *composite*
- **`powTwoLE`** — largest power of two <= n, via a while loop  
  `powTwoLE(n Num) → r Num` · *composite*
- **`quadruple`** — double twice, by passing the `double` function to applyTwice  
  `quadruple(x Num) → r Num` · *composite*
- **`runningMax`** — running maximum of 1..n  
  `runningMax(n Num) → r Stream<Num>` · *composite*
- **`runningSum`** — running sum of 1..n — a stateful stream fold  
  `runningSum(n Num) → r Stream<Num>` · *composite*
- **`secretPeek`** — show a config value and a masked secret — injected at runtime into python  
  `secretPeek() → r Str` · *atomic · python*
- **`siteGreeting`** — greet using an injected config value — needs.kind.alias in a funk body  
  `siteGreeting() → r Str` · *composite*
- **`squares`** — squares of 1..n  
  `squares(n Num) → r Stream<Num>` · *composite*
- **`sumSquares`** — sum of the squares of 1..n (a windowed aggregate)  
  `sumSquares(n Num) → r Num` · *composite*
- **`ticks`** — emit n numbers, one every 200ms — a real-time source, bounded by take  
  `ticks(n Num) → r Stream<Num>` · *composite*
- **`triage`** — a composite that calls createIssue — its needs/effects aggregate up  
  `triage(title Str) → r Json` · *composite*

## funk/std/logic

- **`and`** — logical and  
  `and(a Bool, b Bool) → r Bool` · *atomic · builtin*
- **`not`** — logical not  
  `not(a Bool) → r Bool` · *atomic · builtin*
- **`or`** — logical or  
  `or(a Bool, b Bool) → r Bool` · *atomic · builtin*

## funk/std/maths

- **`abs`** — absolute value  
  `abs(a Num) → r Num` · *atomic · builtin*
- **`add`** — add two numbers  
  `add(a Num, b Num) → r Num` · *atomic · builtin*
- **`div`** — divide a by b  
  `div(a Num, b Num) → r Num` · *atomic · builtin*
- **`double`** — a times two  
  `double(a Num) → r Num` · *atomic · builtin*
- **`eq`** — a equals b  
  `eq(a Num, b Num) → r Bool` · *atomic · builtin*
- **`even?`** — true if a is even  
  `even?(a Num) → r Bool` · *atomic · builtin*
- **`gt`** — a greater than b  
  `gt(a Num, b Num) → r Bool` · *atomic · builtin*
- **`gte`** — a greater than or equal b  
  `gte(a Num, b Num) → r Bool` · *atomic · builtin*
- **`inc`** — a plus one  
  `inc(a Num) → r Num` · *atomic · builtin*
- **`lt`** — a less than b  
  `lt(a Num, b Num) → r Bool` · *atomic · builtin*
- **`lte`** — a less than or equal b  
  `lte(a Num, b Num) → r Bool` · *atomic · builtin*
- **`max`** — the larger of two numbers  
  `max(a Num, b Num) → r Num` · *atomic · builtin*
- **`min`** — the smaller of two numbers  
  `min(a Num, b Num) → r Num` · *atomic · builtin*
- **`mod`** — remainder of a divided by b  
  `mod(a Num, b Num) → r Num` · *atomic · builtin*
- **`mul`** — multiply two numbers  
  `mul(a Num, b Num) → r Num` · *atomic · builtin*
- **`neg`** — negate a number  
  `neg(a Num) → r Num` · *atomic · builtin*
- **`neq`** — a not equal b  
  `neq(a Num, b Num) → r Bool` · *atomic · builtin*
- **`pow`** — a raised to the power b  
  `pow(a Num, b Num) → r Num` · *atomic · builtin*
- **`sqrt`** — square root  
  `sqrt(a Num) → r Num` · *atomic · builtin*
- **`square`** — a squared  
  `square(a Num) → r Num` · *atomic · builtin*
- **`sub`** — subtract b from a  
  `sub(a Num, b Num) → r Num` · *atomic · builtin*

## funk/std/mathx

- **`ceil`** — smallest integer >= a  
  `ceil(a Num) → r Num` · *atomic · python*
- **`clamp`** — clamp x into the inclusive range [lo, hi]  
  `clamp(x Num, lo Num, hi Num) → r Num` · *atomic · python*
- **`cos`** — cosine of a (radians)  
  `cos(a Num) → r Num` · *atomic · python*
- **`exp`** — e raised to the power a  
  `exp(a Num) → r Num` · *atomic · python*
- **`factorial`** — factorial of a non-negative integer a  
  `factorial(a Num) → r Num` · *atomic · python*
- **`floor`** — largest integer <= a  
  `floor(a Num) → r Num` · *atomic · python*
- **`gcd`** — greatest common divisor of a and b  
  `gcd(a Num, b Num) → r Num` · *atomic · python*
- **`log`** — natural logarithm of a  
  `log(a Num) → r Num` · *atomic · python*
- **`log10`** — base-10 logarithm of a  
  `log10(a Num) → r Num` · *atomic · python*
- **`round`** — round a to n decimal places  
  `round(a Num, n Num) → r Num` · *atomic · python*
- **`sin`** — sine of a (radians)  
  `sin(a Num) → r Num` · *atomic · python*
- **`tan`** — tangent of a (radians)  
  `tan(a Num) → r Num` · *atomic · python*

## funk/std/skills

- **`architect`** — turn a task into a concise function design  
  `architect(task Str) → design Str` · *atomic · claude*
- **`generate`** — generate a funk function from a task (architect then programmer)  
  `generate(task Str) → code Str` · *composite*
- **`programmer`** — turn a design into a valid .funk function  
  `programmer(design Str) → code Str` · *atomic · claude*
- **`reflect`** — self-analyze a function against a report; rewrite the source if needed  
  `reflect(code Str, report Str) → code Str` · *atomic · claude*
- **`reviewer`** — review a .funk function for correctness and clarity  
  `reviewer(code Str) → review Str` · *atomic · claude*
- **`tester`** — propose test cases and a verdict for a .funk function  
  `tester(code Str) → verdict Str` · *atomic · claude*

## funk/std/stats

- **`median`** — median of a list of numbers  
  `median(xs List) → r Num` · *atomic · python*
- **`mode`** — most common value in a list  
  `mode(xs List) → r Any` · *atomic · python*
- **`percentile`** — p-th percentile (0-100) of a list, linear interpolation  
  `percentile(p Num, xs List) → r Num` · *atomic · python*
- **`pmax`** — maximum value in a list  
  `pmax(xs List) → r Num` · *atomic · python*
- **`pmin`** — minimum value in a list  
  `pmin(xs List) → r Num` · *atomic · python*
- **`range`** — difference between the max and min of a list  
  `range(xs List) → r Num` · *atomic · python*
- **`stdev`** — sample standard deviation of a list of numbers  
  `stdev(xs List) → r Num` · *atomic · python*
- **`variance`** — sample variance of a list of numbers  
  `variance(xs List) → r Num` · *atomic · python*

## funk/std/stream

- **`filter`** — keep items where f is true (defined in funk)  
  `filter(xs Stream, f Fn) → r Stream` · *composite*
- **`filterCount`** — how many even numbers in 1..n via the funk-defined filter  
  `filterCount(n Num) → r Num` · *composite*
- **`map`** — apply f to each item of a stream (defined in funk)  
  `map(xs Stream, f Fn) → r Stream` · *composite*
- **`mapSum`** — sum of doubles of 1..n via the funk-defined map  
  `mapSum(n Num) → r Num` · *composite*

## funk/std/strings

- **`concat`** — concatenate two strings  
  `concat(a Str, b Str) → r Str` · *atomic · builtin*
- **`length`** — string length  
  `length(a Str) → r Num` · *atomic · builtin*
- **`lower`** — lowercase  
  `lower(a Str) → r Str` · *atomic · builtin*
- **`trim`** — trim whitespace  
  `trim(a Str) → r Str` · *atomic · builtin*
- **`upper`** — uppercase  
  `upper(a Str) → r Str` · *atomic · builtin*

## funk/std/text

- **`capitalize`** — capitalize the first character of a string  
  `capitalize(a Str) → r Str` · *atomic · python*
- **`contains?`** — true if a contains the substring sub  
  `contains?(a Str, sub Str) → r Bool` · *atomic · python*
- **`endsWith?`** — true if a ends with the suffix  
  `endsWith?(a Str, suffix Str) → r Bool` · *atomic · python*
- **`join`** — join a list of strings with a separator  
  `join(xs List, sep Str) → r Str` · *atomic · python*
- **`repeat`** — repeat a string n times  
  `repeat(a Str, n Num) → r Str` · *atomic · python*
- **`replace`** — replace all occurrences of old with new in a  
  `replace(a Str, old Str, new Str) → r Str` · *atomic · python*
- **`reverse`** — reverse the characters of a string  
  `reverse(a Str) → r Str` · *atomic · python*
- **`split`** — split a string by a separator into a list  
  `split(a Str, sep Str) → r List` · *atomic · python*
- **`startsWith?`** — true if a starts with the prefix  
  `startsWith?(a Str, prefix Str) → r Bool` · *atomic · python*
- **`words`** — split a string on whitespace into a list of words  
  `words(a Str) → r List` · *atomic · python*

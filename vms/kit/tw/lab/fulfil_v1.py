"""Fulfilment worker: workflow Fulfil on task queue fulfil."""
import asyncio
from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta

from temporalio import activity, workflow
from temporalio.client import Client
from temporalio.worker import Worker

with workflow.unsafe.imports_passed_through():
    import shop

OPTS = dict(start_to_close_timeout=timedelta(seconds=10))


@activity.defn
def reserve(order: str) -> str:
    return shop.post("/step/reserve", {"run": order})["done"]


@activity.defn
def fraud_check(order: str) -> str:
    """New: the fraud team's check. Not called by the workflow yet."""
    return shop.post("/step/fraud", {"run": order})["done"]


@activity.defn
def charge(order: str) -> str:
    return shop.post("/step/charge", {"run": order})["done"]


@activity.defn
def ship(order: str) -> str:
    return shop.post("/step/ship", {"run": order})["done"]


@workflow.defn
class Fulfil:
    def __init__(self) -> None:
        self.packed = False

    @workflow.signal(name="packed")
    def mark_packed(self) -> None:
        self.packed = True

    @workflow.run
    async def run(self, order: str) -> str:
        await workflow.execute_activity(reserve, order, **OPTS)
        await workflow.execute_activity(charge, order, **OPTS)
        await workflow.wait_condition(lambda: self.packed)   # the warehouse signals "packed"
        await workflow.execute_activity(ship, order, **OPTS)
        return "shipped"


async def main():
    client = await Client.connect("127.0.0.1:7233")
    worker = Worker(client, task_queue="fulfil", workflows=[Fulfil],
                    activities=[reserve, fraud_check, charge, ship],
                    activity_executor=ThreadPoolExecutor(10))
    print("worker polling task queue fulfil", flush=True)
    await worker.run()


if __name__ == "__main__":
    asyncio.run(main())

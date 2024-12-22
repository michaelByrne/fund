import os
import random
from datetime import datetime, timedelta

import psycopg2
import sys
import time
from dateutil.relativedelta import relativedelta
from faker import Faker
from psycopg2.extras import execute_values

# Database connection settings
DB_CONFIG = {
    'dbname': os.getenv("PG_DB"),
    'user': os.getenv("PG_USER"),
    'password': os.getenv("PG_PASS"),
    'host': os.getenv("PG_HOST"),
    'port': os.getenv("PG_PORT")
}

# Constants
BATCH_SIZE = 1000
NUM_DONATIONS = 500
NUM_MEMBERS = 20
NUM_PLANS = 10
PAYMENT_CUTOFF_DATE = datetime.now()
START_DATE = PAYMENT_CUTOFF_DATE - timedelta(days=365)

fake = Faker()


def random_date_within_range():
    return fake.date_time_between(start_date=START_DATE, end_date=PAYMENT_CUTOFF_DATE)


def add_months(date, months):
    return date + relativedelta(months=months)


def batch_insert(cursor, table_name, columns, data, batch_size=BATCH_SIZE):
    """Generic batch insert function"""
    print(f"Inserting {len(data)} records into {table_name}")
    for i in range(0, len(data), batch_size):
        batch = data[i:i + batch_size]
        execute_values(
            cursor,
            f"INSERT INTO {table_name} ({', '.join(columns)}) VALUES %s",
            batch
        )
        print(f"Inserted batch {i // batch_size + 1} of {(len(data) + batch_size - 1) // batch_size}")


def generate_members(num_members):
    return [
        (fake.uuid4(), fake.name(), fake.email(), fake.name(), fake.name(), random_date_within_range())
        for _ in range(num_members)
    ]


def generate_donation_plans(fund_ids, fund_created_map):
    """Generate donation plans only for monthly funds"""
    INTERVAL_UNITS = ['WEEK', 'MONTH']
    unique_plans = {}
    max_attempts = 100
    attempts = 0

    print("Starting donation plan generation...")

    # Filter for only monthly funds
    monthly_fund_ids = [
        fund_id for fund_id in fund_ids
        if fund_created_map[fund_id][1] == "monthly"
    ]

    if not monthly_fund_ids:
        print("No monthly funds found - no plans will be generated")
        return []

    while len(unique_plans) < NUM_PLANS and attempts < max_attempts:
        attempts += 1

        fund_id = random.choice(monthly_fund_ids)
        amount_cents = random.randint(10, 100) * 100
        interval_unit = random.choice(INTERVAL_UNITS)

        key = (amount_cents, interval_unit, fund_id)
        if key not in unique_plans:
            unique_plans[key] = (
                fake.uuid4(),
                f"Plan {len(unique_plans) + 1}",
                fake.uuid4(),
                amount_cents,
                interval_unit,
                random.randint(1, 12),
                True,  # Set active to True for all plans
                datetime.now(),
                datetime.now(),
                fund_id
            )
            print(f"Generated plan {len(unique_plans)} of {NUM_PLANS}")

    plans = list(unique_plans.values())
    if len(plans) < NUM_PLANS:
        print(f"Warning: Only generated {len(plans)} plans instead of {NUM_PLANS}")

    return plans


def random_date_within_range():
    """Generate a timezone-naive datetime within the specified range"""
    # Convert to naive datetime if needed
    start = START_DATE.replace(tzinfo=None)
    end = PAYMENT_CUTOFF_DATE.replace(tzinfo=None)
    return fake.date_time_between(start_date=start, end_date=end)


def distribute_dates_in_range(start_date, end_date, count):
    """Generate evenly distributed dates between start and end date"""
    if start_date.tzinfo:
        start_date = start_date.replace(tzinfo=None)
    if end_date.tzinfo:
        end_date = end_date.replace(tzinfo=None)

    # Calculate the time range
    time_range = (end_date - start_date).total_seconds()

    # Generate timestamps evenly distributed across the range
    timestamps = []
    for i in range(count):
        # Add some randomness while maintaining general distribution
        point = start_date + timedelta(
            seconds=time_range * (i / count) + random.uniform(-time_range / (count * 2), time_range / (count * 2))
        )
        # Ensure we don't go outside our bounds
        point = min(max(point, start_date), end_date)
        timestamps.append(point)

    return sorted(timestamps)


def get_fund_date_range(fund_created_date, fund_expires=None):
    """Get the valid date range for a fund"""
    start_date = fund_created_date.replace(tzinfo=None)
    end_date = (fund_expires or datetime.now()).replace(tzinfo=None)
    return start_date, end_date


def generate_donations(member_ids, fund_created_map, donation_plans, num_donations):
    """Generate donations distributed across fund lifetimes"""
    donations = []

    # First, separate funds by type
    monthly_funds = {
        fund_id: (created, freq)
        for fund_id, (created, freq) in fund_created_map.items()
        if freq == "monthly"
    }
    once_funds = {
        fund_id: (created, freq)
        for fund_id, (created, freq) in fund_created_map.items()
        if freq == "once"
    }

    print(f"Found {len(monthly_funds)} monthly funds and {len(once_funds)} one-time funds")

    # Calculate donations per fund type
    total_funds = len(monthly_funds) + len(once_funds)
    if total_funds == 0:
        raise ValueError("No valid funds found")

    donations_per_fund = num_donations // total_funds

    # Process monthly funds (recurring donations only)
    for fund_id, (fund_created, _) in monthly_funds.items():
        print(f"\nProcessing monthly fund {fund_id}")
        start_date, end_date = get_fund_date_range(fund_created)

        # Get valid plans for this fund
        fund_plans = [p for p in donation_plans if p[9] == fund_id]
        if not fund_plans:
            print(f"Warning: No plans found for monthly fund {fund_id}")
            continue

        # Generate recurring donations spread across the fund's lifetime
        recurring_dates = distribute_dates_in_range(start_date, end_date, donations_per_fund)
        for created_date in recurring_dates:
            plan = random.choice(fund_plans)
            donations.append((
                fake.uuid4(),
                random.choice(member_ids),
                fund_id,
                "666",
                True,
                created_date,
                plan[0],  # plan_id
                True     # recurring
            ))
        print(f"Generated {len(recurring_dates)} recurring donations for monthly fund {fund_id}")

    # Process one-time funds (one-time donations only)
    for fund_id, (fund_created, _) in once_funds.items():
        print(f"\nProcessing one-time fund {fund_id}")
        start_date, end_date = get_fund_date_range(fund_created)

        # Generate one-time donations spread across the fund's lifetime
        one_time_dates = distribute_dates_in_range(start_date, end_date, donations_per_fund)
        for created_date in one_time_dates:
            donations.append((
                fake.uuid4(),
                random.choice(member_ids),
                fund_id,
                "666",
                True,
                created_date,
                None,    # no plan_id
                False   # not recurring
            ))
        print(f"Generated {len(one_time_dates)} one-time donations for fund {fund_id}")

    # Sort all donations by creation date
    donations = sorted(donations, key=lambda x: x[5])
    print(f"\nTotal donations generated: {len(donations)}")

    return donations


def process_donation_payments(cursor, donations, fund_created_map):
    """Generate payments for donations according to their creation dates and plans"""
    total_payments = 0
    current_payments = []
    cutoff_date = datetime.now().replace(tzinfo=None)

    for i, donation in enumerate(donations):
        donation_id, plan_id, created_date, fund_id = donation[0], donation[6], donation[5], donation[2]
        created_date = created_date.replace(tzinfo=None) if created_date.tzinfo else created_date

        if donation[7]:  # recurring donation
            if plan_id:
                cursor.execute(
                    "SELECT interval_unit, amount_cents FROM donation_plan WHERE id = %s",
                    (plan_id,)
                )
                plan_data = cursor.fetchone()

                if plan_data:
                    interval_unit, amount_cents = plan_data
                    next_payment_date = created_date

                    # Generate all payments from creation date until now
                    while next_payment_date <= cutoff_date:
                        current_payments.append((
                            fake.uuid4(),
                            donation_id,
                            amount_cents,
                            "paypal",
                            next_payment_date
                        ))

                        if len(current_payments) >= BATCH_SIZE:
                            batch_insert(
                                cursor,
                                'donation_payment',
                                ['id', 'donation_id', 'amount_cents', 'paypal_payment_id', 'created'],
                                current_payments
                            )
                            total_payments += len(current_payments)
                            current_payments = []

                        # Move to next payment date
                        if interval_unit == "WEEK":
                            next_payment_date += timedelta(weeks=1)
                        elif interval_unit == "MONTH":
                            next_payment_date = add_months(next_payment_date, 1)

        else:  # one-time donation
            # Single payment on donation date
            current_payments.append((
                fake.uuid4(),
                donation_id,
                random.randint(10, 100) * 100,
                "paypal",
                created_date
            ))

            if len(current_payments) >= BATCH_SIZE:
                batch_insert(
                    cursor,
                    'donation_payment',
                    ['id', 'donation_id', 'amount_cents', 'paypal_payment_id', 'created'],
                    current_payments
                )
                total_payments += len(current_payments)
                current_payments = []

        if (i + 1) % 100 == 0:
            print(f"Processed payments for {i + 1} donations. Total payments: {total_payments}")

    # Insert any remaining payments
    if current_payments:
        batch_insert(
            cursor,
            'donation_payment',
            ['id', 'donation_id', 'amount_cents', 'paypal_payment_id', 'created'],
            current_payments
        )
        total_payments += len(current_payments)

    return total_payments


def main():
    if len(sys.argv) != 4:
        print("Usage: python script.py <fund_id_1> <fund_id_2> <fund_id_3>")
        sys.exit(1)

    fund_ids = sys.argv[1:4]
    start_time = time.time()

    try:
        with psycopg2.connect(**DB_CONFIG) as connection:
            with connection.cursor() as cursor:
                print("Starting database seeding process...")

                # Query funds
                cursor.execute(
                    "SELECT id, created, payout_frequency FROM fund WHERE id IN %s",
                    (tuple(fund_ids),)
                )
                funds = cursor.fetchall()

                if len(funds) != 3:
                    raise ValueError("One or more fund IDs not found in the database.")

                # Update fund expiration
                fund_created_map = {fund[0]: (fund[1], fund[2]) for fund in funds}
                for fund_id in fund_created_map:
                    _, payout_frequency = fund_created_map[fund_id]
                    if payout_frequency == "once":
                        cursor.execute("UPDATE fund SET expires = NULL WHERE id = %s", (fund_id,))

                # Insert members
                print("\nGenerating members...")
                members = generate_members(NUM_MEMBERS)
                batch_insert(
                    cursor,
                    'member',
                    ['id', 'bco_name', 'email', 'first_name', 'last_name', 'created'],
                    members
                )

                # Get member IDs
                cursor.execute("SELECT id FROM member")
                member_ids = [row[0] for row in cursor.fetchall()]

                # Insert donation plans
                print("\nGenerating donation plans...")
                donation_plans = generate_donation_plans(fund_ids, fund_created_map)
                batch_insert(
                    cursor,
                    'donation_plan',
                    ['id', 'name', 'paypal_plan_id', 'amount_cents', 'interval_unit',
                     'interval_count', 'active', 'created', 'updated', 'fund_id'],
                    donation_plans
                )

                # Insert donations
                print("\nGenerating donations...")
                donations = generate_donations(member_ids, fund_created_map, donation_plans, NUM_DONATIONS)
                batch_insert(
                    cursor,
                    'donation',
                    ['id', 'donor_id', 'fund_id', 'provider_order_id', 'active',
                     'created', 'donation_plan_id', 'recurring'],
                    donations
                )

                # Process payments
                print("\nProcessing donation payments...")
                total_payments = process_donation_payments(cursor, donations, fund_created_map)

                end_time = time.time()
                print(f"\nDatabase seeding completed successfully!")
                print(f"Total execution time: {end_time - start_time:.2f} seconds")
                print(f"Generated {len(members)} members")
                print(f"Generated {len(donation_plans)} donation plans")
                print(f"Generated {len(donations)} donations")
                print(f"Generated {total_payments} payments")

    except Exception as e:
        print(f"An error occurred: {e}")
        raise


if __name__ == "__main__":
    main()
